package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func runJunk(root string, globs ...string) Result {
	return Run(root, &config.Config{Junk: &config.Junk{Globs: globs}})
}

func TestJunk(t *testing.T) {
	t.Run("matches a basename anywhere under the root", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"notes/deep/.DS_Store": "\x00",
			"keep.md":              "x\n",
		})
		res := runJunk(root, ".DS_Store")
		wantCounts(t, res, 0, 1)
		if got := res.Findings[0].Path; got != "notes/deep/.DS_Store" {
			t.Errorf("got path %q", got)
		}
	})

	t.Run("matches a relative path pattern", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"scratch/a.md": "x\n",
			"memory/a.md":  "x\n",
		})
		wantCounts(t, runJunk(root, "scratch/*.md"), 0, 1)
	})

	t.Run("a star does not cross a slash", func(t *testing.T) {
		root := writeTree(t, map[string]string{"a/b/c.tmp": "x\n"})
		wantCounts(t, runJunk(root, "*/*.tmp"), 0, 0)
	})

	t.Run("a matching directory is reported once and pruned", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"node_modules/pkg/a.js":      "x\n",
			"node_modules/pkg/b.js":      "x\n",
			"node_modules/pkg/deep/c.js": "x\n",
		})
		res := runJunk(root, "node_modules")
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "junk directory")
	})

	t.Run("several matching globs still produce one finding", func(t *testing.T) {
		root := writeTree(t, map[string]string{"notes/scratch.tmp": "x\n"})
		wantCounts(t, runJunk(root, "*.tmp", "notes/*", "scratch.tmp"), 0, 1)
	})

	t.Run(".git is never walked", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			".git/objects/pack.tmp": "x\n",
			"keep.md":               "x\n",
		})
		wantCounts(t, runJunk(root, "*.tmp"), 0, 0)
	})

	t.Run("the config file itself is just a file", func(t *testing.T) {
		root := writeTree(t, map[string]string{".memlint.toml": "[junk]\n"})
		wantCounts(t, runJunk(root, "*.tmp"), 0, 0)
	})
}

func TestJunkReportsUnreadableDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping permission test in short mode")
	}
	root := writeTree(t, map[string]string{"locked/a.md": "x\n"})
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skipf("cannot drop permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	// An unreadable directory is not clean and is not advisory: the invariant
	// could not be evaluated, which is RED so the run fails loudly.
	res := runJunk(root, "*.tmp")
	if res.Red() == 0 {
		t.Skip("running as a user that can read mode 000 directories")
	}
	wantMessage(t, res, "could not verify")
}
