// Package report renders lint results as text or JSON.
package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/frankbesch/memlint/internal/lint"
)

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
	ansiDim    = "\033[2m"
)

// Text writes findings in aligned columns followed by a summary line. On a
// clean run it writes exactly one green line.
func Text(w io.Writer, res lint.Result, color bool) error {
	c := colorizer(color)

	if len(res.Findings) == 0 {
		if res.RulesRun == 0 {
			// Never let an empty config read as a verified repository.
			_, err := fmt.Fprintln(w, c(ansiGreen, "memlint: clean (no rules enabled)"))
			return err
		}
		_, err := fmt.Fprintln(w, c(ansiGreen, fmt.Sprintf(
			"memlint: clean (%s, %s checked)",
			plural(res.RulesRun, "rule"), plural(res.FilesChecked, "file"))))
		return err
	}

	ruleWidth, locWidth := 0, 0
	locs := make([]string, len(res.Findings))
	for i, f := range res.Findings {
		locs[i] = location(f)
		ruleWidth = max(ruleWidth, len(f.Rule))
		locWidth = max(locWidth, len(locs[i]))
	}

	for i, f := range res.Findings {
		sev := c(severityColor(f.Severity), pad(string(f.Severity), 6))
		msg := f.Message
		// The bracketed code makes each finding self-describing: it is the key
		// into docs/findings.md, whose URL closes the report.
		if f.Code != "" {
			msg += " " + c(ansiDim, "["+f.Code+"]")
		}
		line := fmt.Sprintf("%s  %s  %s  %s",
			pad(f.Rule, ruleWidth), sev, pad(locs[i], locWidth), msg)
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " ")); err != nil {
			return err
		}
		// The counterpart line is noise when the message already names it, as
		// the pointer rule's messages do.
		if f.RelatedPath != "" && !strings.Contains(f.Message, f.RelatedPath) {
			if _, err := fmt.Fprintln(w, c(ansiDim, "    counterpart: "+f.RelatedPath)); err != nil {
				return err
			}
		}
		for _, d := range strings.Split(f.Detail, "\n") {
			if d == "" {
				continue
			}
			if _, err := fmt.Fprintln(w, c(ansiDim, "    "+d)); err != nil {
				return err
			}
		}
	}

	// The docs line precedes the summary so that the summary stays the final
	// line — scripts that read it with tail -1 keep working.
	if _, err := fmt.Fprintln(w, c(ansiDim, "docs: "+DocBase)); err != nil {
		return err
	}

	red, yellow := res.Red(), res.Yellow()
	summary := fmt.Sprintf("memlint: %d red, %d yellow", red, yellow)
	tone := ansiYellow
	if red > 0 {
		tone = ansiRed
	}
	_, err := fmt.Fprintln(w, c(tone, summary))
	return err
}

// location renders the path[:line] column, naming the related path only when
// there is no primary line number to show.
func location(f lint.Finding) string {
	if f.Line > 0 {
		return f.Path + ":" + strconv.Itoa(f.Line)
	}
	return f.Path
}

func severityColor(s lint.Severity) string {
	if s == lint.SeverityRed {
		return ansiRed
	}
	return ansiYellow
}

func colorizer(enabled bool) func(code, s string) string {
	if !enabled {
		return func(_, s string) string { return s }
	}
	return func(code, s string) string { return code + s + ansiReset }
}

// pad right-pads to width. Applied before colorizing so escape codes never
// count toward the column width.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
