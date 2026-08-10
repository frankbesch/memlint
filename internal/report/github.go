package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/frankbesch/memlint/internal/lint"
)

// GitHub writes one workflow-command annotation per finding, so findings
// render inline on the pull request diff: ::error for RED, ::warning for
// YELLOW. The summary line follows as plain text for the job log. Never
// colored — GitHub renders annotations itself.
//
// Reference: GitHub Actions "workflow commands" syntax. The escaping rules
// below are theirs, and getting them wrong corrupts the annotation silently,
// which is why escapeData and escapeProp have their own tests.
func GitHub(w io.Writer, res lint.Result) error {
	for _, f := range res.Findings {
		cmd := "error"
		if f.Severity == lint.SeverityYellow {
			cmd = "warning"
		}

		props := "file=" + escapeProp(f.Path)
		if f.Line > 0 {
			props += ",line=" + strconv.Itoa(f.Line)
		}
		props += ",title=" + escapeProp("memlint "+f.Code)

		msg := f.Message
		if f.RelatedPath != "" && !strings.Contains(msg, f.RelatedPath) {
			msg += " (counterpart: " + f.RelatedPath + ")"
		}
		if f.Detail != "" {
			msg += "\n" + f.Detail
		}

		if _, err := fmt.Fprintf(w, "::%s %s::%s\n", cmd, props, escapeData(msg)); err != nil {
			return err
		}
	}

	return summaryLine(w, res)
}

// summaryLine mirrors the text format's final line, uncolored.
func summaryLine(w io.Writer, res lint.Result) error {
	if len(res.Findings) == 0 {
		if res.RulesRun == 0 {
			_, err := fmt.Fprintln(w, "memlint: clean (no rules enabled)")
			return err
		}
		_, err := fmt.Fprintf(w, "memlint: clean (%s, %s checked)\n",
			plural(res.RulesRun, "rule"), plural(res.FilesChecked, "file"))
		return err
	}
	_, err := fmt.Fprintf(w, "memlint: %d red, %d yellow\n", res.Red(), res.Yellow())
	return err
}

// escapeData escapes a workflow command's data portion. Percent first, or the
// escapes themselves would be re-escaped.
func escapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// escapeProp escapes a property value, which additionally cannot carry the
// property separator or the key-value delimiter.
func escapeProp(s string) string {
	s = escapeData(s)
	s = strings.ReplaceAll(s, ",", "%2C")
	s = strings.ReplaceAll(s, ":", "%3A")
	return s
}
