package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func runMirrors(root string, pairs ...[]string) Result {
	return Run(root, &config.Config{Mirrors: &config.Mirrors{Pairs: pairs}})
}

func pair(a, b string) []string { return []string{a, b} }

func TestMirrorsFiles(t *testing.T) {
	t.Run("identical files are silent", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"CLAUDE.md":      "same\n",
			"docs/CLAUDE.md": "same\n",
		})
		wantCounts(t, runMirrors(root, pair("CLAUDE.md", "docs/CLAUDE.md")), 0, 0)
	})

	t.Run("differing files report the divergent byte", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"CLAUDE.md":      "line one\nline two\n",
			"docs/CLAUDE.md": "line one\nline TWO\n",
		})
		res := runMirrors(root, pair("CLAUDE.md", "docs/CLAUDE.md"))
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "differ at byte 14 (line 2, col 6)")
		if got := res.Findings[0].RelatedPath; got != "docs/CLAUDE.md" {
			t.Errorf("got related path %q, want docs/CLAUDE.md", got)
		}
	})

	t.Run("a prefix is still a difference", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"a.md": "one\n",
			"b.md": "one\ntwo\n",
		})
		wantCounts(t, runMirrors(root, pair("a.md", "b.md")), 1, 0)
	})

	t.Run("missing endpoint is red", func(t *testing.T) {
		root := writeTree(t, map[string]string{"CLAUDE.md": "x\n"})
		res := runMirrors(root, pair("CLAUDE.md", "docs/CLAUDE.md"))
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "mirror endpoint does not exist")
	})

	t.Run("file mirrored against a directory is red", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"CLAUDE.md":   "x\n",
			"docs/one.md": "y\n",
		})
		res := runMirrors(root, pair("CLAUDE.md", "docs"))
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "not the same kind")
	})
}

func TestMirrorsDirectories(t *testing.T) {
	t.Run("identical trees are silent, including nested files", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":       "a\n",
			"left/deep/b.md":  "b\n",
			"right/a.md":      "a\n",
			"right/deep/b.md": "b\n",
		})
		wantCounts(t, runMirrors(root, pair("left", "right")), 0, 0)
	})

	t.Run("a member missing on one side is red", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":  "a\n",
			"left/b.md":  "b\n",
			"right/a.md": "a\n",
		})
		res := runMirrors(root, pair("left", "right"))
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "present in left but missing from right")
	})

	t.Run("a member missing on the other side is also red", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":  "a\n",
			"right/a.md": "a\n",
			"right/c.md": "c\n",
		})
		res := runMirrors(root, pair("left", "right"))
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "present in right but missing from left")
	})

	t.Run("a differing member is red", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":  "a\n",
			"right/a.md": "different\n",
		})
		wantCounts(t, runMirrors(root, pair("left", "right")), 1, 0)
	})

	t.Run("only markdown participates", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":      "a\n",
			"left/notes.txt": "only on the left, and not markdown\n",
			"right/a.md":     "a\n",
		})
		wantCounts(t, runMirrors(root, pair("left", "right")), 0, 0)
	})

	t.Run("a nested .git directory is skipped", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"left/a.md":           "a\n",
			"left/.git/config.md": "vcs internals, not content\n",
			"right/a.md":          "a\n",
		})
		wantCounts(t, runMirrors(root, pair("left", "right")), 0, 0)
	})
}

// A symlinked directory must not be walked into: following it would let a link
// widen a check to files outside the tree memlint was pointed at.
func TestMirrorsDoesNotFollowSymlinkedDirectories(t *testing.T) {
	root := writeTree(t, map[string]string{
		"left/a.md":    "a\n",
		"right/a.md":   "a\n",
		"outside/x.md": "not part of either side\n",
	})
	link := filepath.Join(root, "left", "linked")
	if err := os.Symlink(filepath.Join(root, "outside"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	wantCounts(t, runMirrors(root, pair("left", "right")), 0, 0)
}

// A configured endpoint that resolves outside the root must be refused rather
// than compared.
func TestMirrorsRefusesSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, map[string]string{"a.md": "x\n"})
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "escape.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res := runMirrors(root, pair("a.md", "escape.md"))
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "escapes the repository root")
}
