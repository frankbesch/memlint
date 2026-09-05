package report

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/lint"
)

func TestGitHubAnnotations(t *testing.T) {
	res := lint.Result{
		Findings: []lint.Finding{
			{
				Rule: "pointers", Code: "pointers/dead-ref", Severity: lint.SeverityRed,
				Path: "memory/index.md", RelatedPath: "memory/missing.md", Line: 7,
				Message: "dead reference: memory/missing.md does not exist",
			},
			{
				Rule: "tokens", Code: "tokens/no-match", Severity: lint.SeverityYellow,
				Path:    "notes/*.md",
				Message: "watch glob matched no files",
				Detail:  "a stale watch glob is a budget check that silently never runs",
			},
		},
		RulesRun: 2,
	}

	var b strings.Builder
	if err := GitHub(&b, res); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 2 annotations + summary, got %d lines:\n%s", len(lines), b.String())
	}

	wantRed := "::error file=memory/index.md,line=7,title=memlint pointers/dead-ref::" +
		"dead reference: memory/missing.md does not exist"
	if lines[0] != wantRed {
		t.Errorf("RED annotation:\ngot  %q\nwant %q", lines[0], wantRed)
	}

	// The YELLOW finding's detail rides along, newline-escaped, and its path is
	// a glob whose characters must survive property escaping.
	wantYellow := "::warning file=notes/*.md,title=memlint tokens/no-match::" +
		"watch glob matched no files%0Aa stale watch glob is a budget check that silently never runs"
	if lines[1] != wantYellow {
		t.Errorf("YELLOW annotation:\ngot  %q\nwant %q", lines[1], wantYellow)
	}

	if lines[2] != "memlint: 1 red, 1 yellow" {
		t.Errorf("summary: got %q", lines[2])
	}
}

func TestGitHubCleanRuns(t *testing.T) {
	var b strings.Builder
	if err := GitHub(&b, lint.Result{RulesRun: 2, FilesChecked: 5}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "memlint: clean (2 rules, 5 files checked)\n" {
		t.Errorf("got %q", got)
	}

	b.Reset()
	if err := GitHub(&b, lint.Result{}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "memlint: clean (no rules enabled)\n" {
		t.Errorf("no-rules run: got %q", got)
	}
}

// The escaping rules come from GitHub's workflow-command syntax. Ordering
// matters: percent must be escaped first or the other escapes get re-escaped.
func TestWorkflowCommandEscaping(t *testing.T) {
	if got := escapeData("50% done\r\nnext"); got != "50%25 done%0D%0Anext" {
		t.Errorf("escapeData: got %q", got)
	}
	if got := escapeProp("a,b:c%d"); got != "a%2Cb%3Ac%25d" {
		t.Errorf("escapeProp: got %q", got)
	}
}

// INFO findings are ::notice annotations, and an INFO-only run keeps the
// clean summary: nothing was violated.
func TestGitHubInfoIsNotice(t *testing.T) {
	res := lint.Result{RulesRun: 1, FilesChecked: 2, Findings: []lint.Finding{{
		Rule: "append_only", Code: "append_only/rotated", Severity: lint.SeverityInfo,
		Path: "memory/decisions.md", RelatedPath: "memory/archive/vol1.md", Line: 5,
		Message: "rotated → memory/archive/vol1.md (3 lines moved verbatim)",
	}}}
	var b strings.Builder
	if err := GitHub(&b, res); err != nil {
		t.Fatal(err)
	}
	want := "::notice file=memory/decisions.md,line=5,title=memlint append_only/rotated::" +
		"rotated → memory/archive/vol1.md (3 lines moved verbatim)\n" +
		"memlint: clean (1 rule, 2 files checked)\n"
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
