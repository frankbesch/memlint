package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree materializes a temporary repository from a path -> content map.
// Paths are slash-separated and relative to the returned root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		writeFile(t, root, rel, content)
	}
	return root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// counts returns the RED and YELLOW totals of a result.
func counts(res Result) (red, yellow int) {
	return res.Red(), res.Yellow()
}

// wantCounts fails unless the result has exactly the given severity split,
// printing every finding when it does not.
func wantCounts(t *testing.T, res Result, red, yellow int) {
	t.Helper()
	gotRed, gotYellow := counts(res)
	if gotRed != red || gotYellow != yellow {
		t.Errorf("got %d red / %d yellow, want %d red / %d yellow\n%s",
			gotRed, gotYellow, red, yellow, dump(res))
	}
}

// wantMessage fails unless some finding's message contains substr.
func wantMessage(t *testing.T, res Result, substr string) {
	t.Helper()
	for _, f := range res.Findings {
		if strings.Contains(f.Message, substr) {
			return
		}
	}
	t.Errorf("no finding with message containing %q\n%s", substr, dump(res))
}

func dump(res Result) string {
	if len(res.Findings) == 0 {
		return "  (no findings)"
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString("  " + string(f.Severity) + " " + f.Rule + " " + f.Path)
		if f.RelatedPath != "" {
			b.WriteString(" -> " + f.RelatedPath)
		}
		b.WriteString(": " + f.Message + "\n")
	}
	return b.String()
}
