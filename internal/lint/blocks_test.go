package lint

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

const (
	blockStart = "<!-- AGENT:START -->"
	blockEnd   = "<!-- AGENT:END -->"
)

// runBlocks evaluates only the blocks rule against root with the test markers.
func runBlocks(root string, files ...string) Result {
	return Run(root, &config.Config{Blocks: &config.Blocks{
		Files: files, Start: blockStart, End: blockEnd,
	}})
}

func TestBlocks(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantRed  int
		wantMsg  string
		wantLine int
	}{
		{
			name:    "well-formed block",
			content: "# Notes\n\n<!-- AGENT:START -->\nagent region\n<!-- AGENT:END -->\n\nhuman prose\n",
			wantRed: 0,
		},
		{
			name:    "block opened and closed on one line",
			content: "<!-- AGENT:START --> generated <!-- AGENT:END -->\n",
			wantRed: 0,
		},
		{
			name:    "indented markers still count",
			content: "> <!-- AGENT:START -->\n> region\n> <!-- AGENT:END -->\n",
			wantRed: 0,
		},
		{
			name:    "no markers at all",
			content: "# Notes\n\njust prose\n",
			wantRed: 1, wantMsg: "ownership block missing",
		},
		{
			name:    "start without end",
			content: "# Notes\n\n<!-- AGENT:START -->\nagent region\n",
			wantRed: 1, wantMsg: "unterminated", wantLine: 3,
		},
		{
			name:    "end without start",
			content: "agent region\n<!-- AGENT:END -->\n",
			wantRed: 1, wantMsg: "end marker without a start marker", wantLine: 2,
		},
		{
			name:    "duplicate start marker",
			content: "<!-- AGENT:START -->\na\n<!-- AGENT:END -->\n<!-- AGENT:START -->\nb\n",
			wantRed: 1, wantMsg: "duplicate start marker", wantLine: 4,
		},
		{
			name:    "duplicate end marker",
			content: "<!-- AGENT:START -->\na\n<!-- AGENT:END -->\n<!-- AGENT:END -->\n",
			wantRed: 1, wantMsg: "duplicate end marker", wantLine: 4,
		},
		{
			name:    "end before start",
			content: "<!-- AGENT:END -->\nmiddle\n<!-- AGENT:START -->\n",
			wantRed: 1, wantMsg: "end marker precedes start marker", wantLine: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"AGENTS.md": tc.content})
			res := runBlocks(root, "AGENTS.md")
			wantCounts(t, res, tc.wantRed, 0)
			if tc.wantMsg != "" {
				wantMessage(t, res, tc.wantMsg)
			}
			if tc.wantLine != 0 && res.Findings[0].Line != tc.wantLine {
				t.Errorf("got line %d, want %d", res.Findings[0].Line, tc.wantLine)
			}
		})
	}
}

// A file the config declares as block-carrying must exist. Its absence is a
// violated contract, not an unverifiable one.
func TestBlocksMissingFileIsRed(t *testing.T) {
	root := writeTree(t, map[string]string{"other.md": "x\n"})
	res := runBlocks(root, "AGENTS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "does not exist")
}

// Exactly one finding per file: a file with several structural problems must
// not bury the report, and the first problem is the one to fix first.
func TestBlocksOneFindingPerFile(t *testing.T) {
	root := writeTree(t, map[string]string{
		"AGENTS.md": "<!-- AGENT:START -->\n<!-- AGENT:START -->\n<!-- AGENT:END -->\n<!-- AGENT:END -->\n",
	})
	res := runBlocks(root, "AGENTS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "duplicate start marker")
}

// The duplicate-marker details point back at the first occurrence, so the
// report shows both halves of the ambiguity.
func TestBlocksDuplicateDetailNamesFirstOccurrence(t *testing.T) {
	root := writeTree(t, map[string]string{
		"AGENTS.md": "<!-- AGENT:START -->\na\n<!-- AGENT:END -->\n<!-- AGENT:START -->\n",
	})
	res := runBlocks(root, "AGENTS.md")
	wantCounts(t, res, 1, 0)
	if d := res.Findings[0].Detail; !strings.Contains(d, "line 1") {
		t.Errorf("detail should name the first start marker's line, got %q", d)
	}
}

func TestMarkerOffsets(t *testing.T) {
	content := "a MARK b MARK\nMARK"
	got := markerOffsets(content, "MARK")
	want := []int{2, 9, 14}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("offset[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if offs := markerOffsets("aaaa", "aa"); len(offs) != 2 {
		t.Errorf("overlapping matches must not double-count: got %v", offs)
	}
}
