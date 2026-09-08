package lint

import (
	"fmt"
	"sort"
	"strings"
)

// Severity is the weight of a finding. RED means an invariant was evaluated
// and violated, or could not be evaluated at all. YELLOW is advisory. INFO
// is a receipt: something happened that a reader should see, and it passed.
type Severity string

const (
	// SeverityRed marks a violated invariant. Any RED fails the run.
	SeverityRed Severity = "RED"
	// SeverityYellow marks an advisory finding. YELLOW fails only under --strict.
	SeverityYellow Severity = "YELLOW"
	// SeverityInfo marks a verified event worth a line — an append-only log
	// rotated with its content intact. INFO never fails the run, not even
	// under --strict, and a run with only INFO findings is still clean.
	SeverityInfo Severity = "INFO"
)

// rank orders severities by weight. Sorting on the string would put RED after
// YELLOW, which is backwards.
func (s Severity) rank() int {
	switch s {
	case SeverityRed:
		return 0
	case SeverityYellow:
		return 1
	}
	return 2
}

// Finding is one violated or unverifiable invariant.
type Finding struct {
	Rule string `json:"rule"`
	// Code is the finding's stable machine identity, "<rule>/<kind>"
	// (pointers/dead-ref, blocks/unterminated, tokens/no-match, ...). Messages
	// may be reworded; codes may not. Additions are non-breaking; renaming or
	// removing one bumps report.SchemaVersion.
	Code        string   `json:"code"`
	Severity    Severity `json:"severity"`
	Path        string   `json:"path"`
	RelatedPath string   `json:"related_path,omitempty"`
	Line        int      `json:"line,omitempty"`
	Message     string   `json:"message"`
	Detail      string   `json:"detail,omitempty"`
}

// Result is the outcome of a full run.
type Result struct {
	Findings []Finding
	// RulesRun counts enabled rules. Zero means nothing was verified.
	RulesRun int
	// FilesChecked counts distinct paths any rule actually inspected.
	FilesChecked int
	// Tree is the fingerprint of the tree this result describes (see
	// Fingerprint). Empty when the caller did not compute one.
	Tree string
}

// ExpectTree records a RED tree/moved finding when the result's fingerprint
// does not match want: the receipt names a different tree than the one just
// judged. No-op when the trees match.
func (r *Result) ExpectTree(want string) {
	if TreeMatches(r.Tree, want) {
		return
	}
	r.Findings = append(r.Findings, Finding{
		Rule: "tree", Code: "tree/moved", Severity: SeverityRed, Path: ".",
		Message: fmt.Sprintf("tree moved: fingerprint %s does not match expected %s", ShortTree(r.Tree), ShortTree(strings.ToLower(strings.TrimSpace(want)))),
		Detail:  "the receipt was recorded against a different tree; re-run check on the tree you mean to act on",
	})
	sortFindings(r.Findings)
}

// Red counts RED findings.
func (r Result) Red() int { return r.count(SeverityRed) }

// Yellow counts YELLOW findings.
func (r Result) Yellow() int { return r.count(SeverityYellow) }

// Info counts INFO findings. They never affect the exit code.
func (r Result) Info() int { return r.count(SeverityInfo) }

// Clean reports whether nothing was violated: no RED and no YELLOW. INFO
// findings do not count against it.
func (r Result) Clean() bool { return r.Red() == 0 && r.Yellow() == 0 }

func (r Result) count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// sortFindings imposes a total order so output is byte-stable across runs and
// platforms. Filesystem walk order is not guaranteed, so this is load-bearing.
func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Severity.rank() != b.Severity.rank() {
			return a.Severity.rank() < b.Severity.rank()
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		// Line before related path: several findings in one file should read in
		// the order a reader would scroll through them.
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RelatedPath != b.RelatedPath {
			return a.RelatedPath < b.RelatedPath
		}
		return a.Message < b.Message
	})
}
