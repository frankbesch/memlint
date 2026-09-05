package report

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/lint"
)

func render(t *testing.T, res lint.Result, color bool) string {
	t.Helper()
	var b strings.Builder
	if err := Text(&b, res, color); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestTextCleanRun(t *testing.T) {
	got := render(t, lint.Result{RulesRun: 3, FilesChecked: 12}, false)
	if got != "memlint: clean (3 rules, 12 files checked)\n" {
		t.Errorf("got %q", got)
	}
}

// "clean" must never imply "verified" when nothing was verified.
func TestTextCleanRunWithNoRules(t *testing.T) {
	got := render(t, lint.Result{RulesRun: 0}, false)
	if !strings.Contains(got, "no rules enabled") {
		t.Errorf("got %q", got)
	}
}

func TestTextSingularCounts(t *testing.T) {
	got := render(t, lint.Result{RulesRun: 1, FilesChecked: 1}, false)
	if !strings.Contains(got, "(1 rule, 1 file checked)") {
		t.Errorf("got %q", got)
	}
}

func TestTextFinding(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{{
		Rule: "mirrors", Severity: lint.SeverityRed,
		Path: "CLAUDE.md", RelatedPath: "docs/CLAUDE.md",
		Message: "mirrored files differ at byte 14",
		Detail:  "CLAUDE.md is 20 bytes, docs/CLAUDE.md is 21 bytes",
	}}}
	got := render(t, res, false)

	for _, want := range []string{
		"mirrors  RED",
		"CLAUDE.md",
		"mirrored files differ at byte 14",
		"    counterpart: docs/CLAUDE.md",
		"    CLAUDE.md is 20 bytes",
		"memlint: 1 red, 0 yellow",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestTextLineNumberInLocation(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{{
		Rule: "pointers", Severity: lint.SeverityRed,
		Path: "memory/index.md", Line: 7, Message: "dead reference: memory/x.md does not exist",
	}}}
	if got := render(t, res, false); !strings.Contains(got, "memory/index.md:7") {
		t.Errorf("output should carry the line number:\n%s", got)
	}
}

// The counterpart line is redundant when the message already names it, as
// pointer findings do.
func TestTextSuppressesRedundantCounterpart(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{{
		Rule: "pointers", Severity: lint.SeverityRed,
		Path: "memory/index.md", RelatedPath: "memory/x.md", Line: 7,
		Message: "dead reference: memory/x.md does not exist",
	}}}
	if got := render(t, res, false); strings.Contains(got, "counterpart:") {
		t.Errorf("counterpart line should be suppressed:\n%s", got)
	}
}

func TestTextColor(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{
		{Rule: "junk", Severity: lint.SeverityYellow, Path: "a.tmp", Message: "junk"},
	}}

	if got := render(t, res, false); strings.Contains(got, "\033") {
		t.Errorf("color disabled but escapes present: %q", got)
	}
	colored := render(t, res, true)
	if !strings.Contains(colored, ansiYellow) || !strings.Contains(colored, ansiReset) {
		t.Errorf("color enabled but no escapes present: %q", colored)
	}
}

// Escape codes must not be counted as column width, or colored output would
// misalign against uncolored output.
func TestTextColumnsAlignRegardlessOfColor(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{
		{Rule: "junk", Severity: lint.SeverityYellow, Path: "a.tmp", Message: "one"},
		{Rule: "append_only", Severity: lint.SeverityRed, Path: "b.md", Message: "two"},
	}}
	plain := strings.Split(render(t, res, false), "\n")
	colored := strings.Split(render(t, res, true), "\n")

	for i := range plain {
		if plain[i] == "" {
			continue
		}
		stripped := strings.NewReplacer(
			ansiRed, "", ansiYellow, "", ansiGreen, "", ansiDim, "", ansiReset, "",
		).Replace(colored[i])
		if stripped != plain[i] {
			t.Errorf("line %d differs once color is stripped:\n%q\n%q", i, stripped, plain[i])
		}
	}
}

// An INFO finding is rendered green and does not turn a clean run into a
// red/yellow summary: the run is still clean, and the summary line says so.
func TestTextInfoFindingKeepsCleanSummary(t *testing.T) {
	res := lint.Result{RulesRun: 1, FilesChecked: 2, Findings: []lint.Finding{{
		Rule: "append_only", Code: "append_only/rotated", Severity: lint.SeverityInfo,
		Path: "memory/decisions.md", RelatedPath: "memory/archive/vol1.md",
		Message: "rotated → memory/archive/vol1.md (100 lines moved verbatim)",
	}}}
	got := render(t, res, false)
	for _, want := range []string{
		"append_only  INFO    memory/decisions.md  rotated → memory/archive/vol1.md (100 lines moved verbatim) [append_only/rotated]",
		"docs: " + DocBase,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if last := lines[len(lines)-1]; last != "memlint: clean (1 rule, 2 files checked)" {
		t.Errorf("summary must stay the clean line, got %q", last)
	}
	if strings.Contains(got, "counterpart:") {
		t.Errorf("counterpart line is redundant when the message names it:\n%s", got)
	}
	colored := render(t, res, true)
	if !strings.Contains(colored, ansiGreen+"INFO") {
		t.Errorf("INFO must be green:\n%q", colored)
	}
}

// INFO alongside RED: the red summary is unchanged in shape.
func TestTextInfoDoesNotChangeRedSummary(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{
		{Rule: "junk", Code: "junk/match", Severity: lint.SeverityRed, Path: "a.tmp", Message: "junk"},
		{Rule: "append_only", Code: "append_only/rotated", Severity: lint.SeverityInfo, Path: "b.md", Message: "rotated"},
	}}
	got := render(t, res, false)
	if !strings.HasSuffix(got, "memlint: 1 red, 0 yellow\n") {
		t.Errorf("summary shape must not change, got:\n%s", got)
	}
}
