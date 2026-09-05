package lint

import (
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func runIDs(root string, cfg *config.IDs) Result {
	return Run(root, &config.Config{IDs: cfg})
}

func TestIDsUniqueIsClean(t *testing.T) {
	root := writeTree(t, map[string]string{
		"memory/decisions.md": "# log\n\nD-001 | a\nD-003 | b (D-002 withdrawn)\nSee D-001 mid-line.\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"memory/decisions.md"}})
	wantCounts(t, res, 0, 0)
	if res.FilesChecked != 1 {
		t.Errorf("FilesChecked = %d, want 1", res.FilesChecked)
	}
}

func TestIDsDuplicateInOneFile(t *testing.T) {
	root := writeTree(t, map[string]string{
		"memory/decisions.md": "D-101 | a\nD-102 | session A\nD-102 | session B\nD-103 | c\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"memory/decisions.md"}})
	wantCounts(t, res, 1, 0)
	f := res.Findings[0]
	if f.Code != "ids/duplicate" || f.Rule != "ids" {
		t.Errorf("got %s %s, want ids ids/duplicate", f.Rule, f.Code)
	}
	if f.Path != "memory/decisions.md" || f.Line != 3 {
		t.Errorf("again should be memory/decisions.md:3, got %s:%d", f.Path, f.Line)
	}
	if f.RelatedPath != "memory/decisions.md" {
		t.Errorf("related path should be the first occurrence's file, got %q", f.RelatedPath)
	}
	if want := "duplicate id D-102: first at memory/decisions.md:2"; f.Message != want {
		t.Errorf("message %q, want %q", f.Message, want)
	}
}

// Three occurrences are two findings, each against the first.
func TestIDsTripleIsTwoFindings(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "D-001 | x\nD-001 | y\nD-001 | z\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"a.md"}})
	wantCounts(t, res, 2, 0)
	for _, f := range res.Findings {
		if f.Message != "duplicate id D-001: first at a.md:1" {
			t.Errorf("every duplicate cites the first occurrence, got %q", f.Message)
		}
	}
}

// Uniqueness spans all listed files. Literal entries resolve before glob
// matches, so the literal's occurrence is "first" regardless of walk order.
func TestIDsDuplicateAcrossFilesLiteralIsFirst(t *testing.T) {
	root := writeTree(t, map[string]string{
		"memory/decisions.md":      "D-050 | re-registered\nD-101 | a\n",
		"memory/archive/vol1.md":   "D-001 | a\nD-050 | original\n",
		"memory/archive/vol0.md":   "D-000 | prehistory\n",
		"memory/archive/notes.txt": "D-050 not scanned: not matched by the glob\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"memory/decisions.md", "memory/archive/*.md"}})
	wantCounts(t, res, 1, 0)
	f := res.Findings[0]
	if f.Path != "memory/archive/vol1.md" || f.Line != 2 {
		t.Errorf("again should be memory/archive/vol1.md:2, got %s:%d", f.Path, f.Line)
	}
	if f.Message != "duplicate id D-050: first at memory/decisions.md:1" {
		t.Errorf("got %q", f.Message)
	}
}

// Only a match at column 1 is an id; the default pattern's ^ is redundant
// with that rule, and a pattern without ^ is anchored the same way.
func TestIDsMatchMustStartTheLine(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "D-001 | a\n  D-001 | indented, not an id\nsee D-001 again\n- D-001 | bullet\n",
	})
	wantCounts(t, runIDs(root, &config.IDs{Files: []string{"a.md"}}), 0, 0)
	wantCounts(t, runIDs(root, &config.IDs{Files: []string{"a.md"}, Pattern: `(D-\d{3})`}), 0, 0)
}

// The default pattern needs the entry delimiter: a wrapped continuation line
// that begins with a cited id is not an entry. The bare pattern, opted into,
// reads it as one — which is exactly the false positive the default avoids.
func TestIDsDefaultPatternIgnoresWrappedCitations(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "D-101 | ruled; supersedes\nD-102). Next step is\nD-102 | the real entry\n",
	})
	wantCounts(t, runIDs(root, &config.IDs{Files: []string{"a.md"}}), 0, 0)
	res := runIDs(root, &config.IDs{Files: []string{"a.md"}, Pattern: `^(D-\d{3})`})
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "duplicate id D-102: first at a.md:2")
}

func TestIDsCustomPattern(t *testing.T) {
	root := writeTree(t, map[string]string{
		"lessons.md": "L-1 | a\nL-2 | b\nL-2 | c\nD-001 | not this pattern\nD-001 | nor this\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"lessons.md"}, Pattern: `^(L-\d+)`})
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "duplicate id L-2: first at lessons.md:2")

	// No capture group: the whole match is the id.
	res = runIDs(root, &config.IDs{Files: []string{"lessons.md"}, Pattern: `^L-\d+`})
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "duplicate id L-2")
}

// The id is the capture group, so a longer line prefix does not leak into
// it, and CRLF endings do not either.
func TestIDsCaptureGroupAndCRLF(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "D-001 | a\r\nD-002 | b\r\nD-001 | c\r\n",
	})
	res := runIDs(root, &config.IDs{Files: []string{"a.md"}})
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "duplicate id D-001: first at a.md:1")
}

func TestIDsSourceProblems(t *testing.T) {
	root := writeTree(t, map[string]string{"memory/decisions.md": "D-001\n"})

	res := runIDs(root, &config.IDs{Files: []string{"memory/decisions.md", "memory/gone.md"}})
	wantCounts(t, res, 1, 0)
	if !hasFinding(res, "ids", "memory/gone.md", "does not exist") {
		t.Errorf("missing literal source must be RED ids/missing-source:\n%s", dump(res))
	}

	res = runIDs(root, &config.IDs{Files: []string{"memory/decisions.md", "memory/archive/*.md"}})
	wantCounts(t, res, 0, 1)
	if !hasFinding(res, "ids", "memory/archive/*.md", "matched no files") {
		t.Errorf("zero-match glob must be YELLOW ids/no-match:\n%s", dump(res))
	}
}
