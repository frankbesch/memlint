package lint

import (
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

// --changed narrows a run to findings that touch changed files: modified,
// staged, or untracked. Config-level findings (a glob that matches nothing)
// still surface, since dropping them would be a silent skip.
func TestRunChangedFiltersToChangedFiles(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{
		"a.md":     "D-001 | a\nD-001 | dup in unchanged file\n",
		"b.md":     "D-002 | b\nD-002 | dup in changed file\n",
		"index.md": "see memory/gone.md\n",
	})
	writeFile(t, root, "b.md", "D-002 | b\nD-002 | dup in changed file\nedited\n")
	writeFile(t, root, "new.md", "D-003 | c\nD-003 | dup in untracked file\n")

	changed, err := ChangedFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"b.md", "new.md"} {
		if !changed[want] {
			t.Errorf("changed set lacks %s: %v", want, changed)
		}
	}
	if changed["a.md"] || changed["index.md"] {
		t.Errorf("unchanged files must not be in the set: %v", changed)
	}

	cfg := &config.Config{
		IDs:      &config.IDs{Files: []string{"*.md"}},
		Pointers: &config.Pointers{Files: []string{"index.md", "notes/*.md"}, Roots: []string{"memory"}},
	}
	full := Run(root, cfg)
	wantCounts(t, full, 4, 1)

	res := RunChanged(root, cfg, changed)
	wantCounts(t, res, 2, 1)
	if hasFinding(res, "ids", "a.md", "duplicate") || hasFinding(res, "pointers", "index.md", "dead") {
		t.Errorf("findings in unchanged files must be filtered:\n%s", dump(res))
	}
	if !hasFinding(res, "pointers", "notes/*.md", "matched no files") {
		t.Errorf("config-level findings must survive the filter:\n%s", dump(res))
	}
	if res.FilesChecked != 2 {
		t.Errorf("FilesChecked = %d, want 2 (only changed files count)", res.FilesChecked)
	}
}

func TestChangedFilesNeedsGit(t *testing.T) {
	requireGit(t)
	if _, err := ChangedFiles(t.TempDir()); err == nil {
		t.Error("a directory that is not a git repository must be an error")
	}
}
