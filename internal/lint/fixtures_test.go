package lint

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", name)
}

func runFixture(t *testing.T, name string) Result {
	t.Helper()
	root := fixture(t, name)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("loading %s config: %v", name, err)
	}
	return Run(root, cfg)
}

// The acceptance criterion: fixture-broken has exactly nine planted RED
// defects and four planted YELLOW ones. The count is the test -- if a rule
// starts over-reporting or silently stops reporting, this fails.
func TestFixtureBroken(t *testing.T) {
	res := runFixture(t, "fixture-broken")
	wantCounts(t, res, 9, 4)

	want := []struct{ rule, path, message string }{
		{"blocks", "AGENTS.md", "unterminated"},
		{"blocks", "docs/generated.md", "duplicate start marker"},
		{"mirrors", "CLAUDE.md", "mirrored files differ"},
		{"mirrors", "sync/left/a.md", "mirrored files differ"},
		{"mirrors", "sync/left/b.md", "missing from sync/right"},
		{"pointers", "memory/index.md", "dead reference: memory/missing.md"},
		{"pointers", "memory/index.md", "dead reference: docs/nope.md"},
		{"pointers", "memory/index.md", "dead reference: memory/gone-anchored.md does not exist (referenced as memory/gone-anchored.md#section)"},
		{"pointers", "notes/*.md", "files glob matched no files"},
		{"junk", "notes/scratch.tmp", `junk file matches "*.tmp"`},
		{"tokens", "memory/big.md", "estimated tokens exceeds hard limit"},
		{"tokens", "memory/medium.md", "estimated tokens exceeds budget"},
		{"tokens", "notes/missing/*.md", "watch glob matched no files"},
	}
	for _, w := range want {
		if !hasFinding(res, w.rule, w.path, w.message) {
			t.Errorf("missing expected finding: %s %s %q\ngot:\n%s",
				w.rule, w.path, w.message, dump(res))
		}
	}
}

// Every reference form the fixture plants as a non-finding must stay silent.
// Load-bearing since v0.6 made anchors checkable: memory/multi-hash.md#a#b
// (more than one "#" is not a path+anchor) and the URL with a fragment —
// neither target exists, so a skip regression over-reports here.
// memory/missing.md#section pins dedup with its bare form: exactly one
// missing.md finding, which TestFixtureBroken's exact counts already enforce.
func TestFixtureBrokenSkipsAreNotReported(t *testing.T) {
	res := runFixture(t, "fixture-broken")
	for _, f := range res.Findings {
		for _, skipped := range []string{
			"reading/daily",
			"example.com",
			"reviews/",
			"multi-hash",
			"memory/existing.md",
		} {
			if strings.Contains(f.Message, skipped) || strings.Contains(f.RelatedPath, skipped) {
				t.Errorf("reference that must be skipped was reported: %s %s: %s",
					f.Rule, f.Path, f.Message)
			}
		}
	}
}

func TestFixtureClean(t *testing.T) {
	res := runFixture(t, "fixture-clean")
	wantCounts(t, res, 0, 0)
	if res.RulesRun == 0 {
		t.Error("fixture-clean must actually run rules, otherwise clean means nothing")
	}
}

func TestFixtureYellow(t *testing.T) {
	res := runFixture(t, "fixture-yellow")
	if res.Red() != 0 {
		t.Errorf("fixture-yellow must have no RED findings:\n%s", dump(res))
	}
	if res.Yellow() == 0 {
		t.Error("fixture-yellow must have at least one YELLOW finding")
	}
}

func hasFinding(res Result, rule, path, message string) bool {
	for _, f := range res.Findings {
		if f.Rule == rule && f.Path == path && strings.Contains(f.Message, message) {
			return true
		}
	}
	return false
}

// copyFixture materializes a fixture into a temp dir, so a test can put git
// state around it that the committed tree cannot carry.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := fixture(t, name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", name, err)
	}
	return dst
}

// fixture-rotated needs git state: the pre-rotation log committed, the
// rotated tree in the working copy with the archive untracked. Built here.
func TestFixtureRotated(t *testing.T) {
	requireGit(t)

	build := func(t *testing.T) string {
		root := copyFixture(t, "fixture-rotated")
		live := filepath.Join(root, "memory", "decisions.md")
		rotated, err := os.ReadFile(live)
		if err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(root, "memory", "archive", "decisions-vol1.md")
		vol, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		pre, err := os.ReadFile(filepath.Join(root, "baseline", "decisions.md"))
		if err != nil {
			t.Fatal(err)
		}
		// Commit the pre-rotation state: the live log as it was, no archive.
		os.Remove(archive)
		os.RemoveAll(filepath.Join(root, "baseline"))
		if err := os.WriteFile(live, pre, 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, root, "init", "-q", "-b", "main")
		commit(t, root, "pre-rotation")
		// Now the rotation, uncommitted.
		if err := os.WriteFile(live, rotated, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archive, vol, 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("rotation is one INFO", func(t *testing.T) {
		root := build(t)
		cfg, err := config.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res := Run(root, cfg)
		wantRotated(t, res, "memory/decisions.md", "memory/archive/decisions-vol1.md", 3)
	})

	t.Run("altered moved line is the plain RED", func(t *testing.T) {
		root := build(t)
		archive := filepath.Join(root, "memory", "archive", "decisions-vol1.md")
		data, _ := os.ReadFile(archive)
		tampered := strings.Replace(string(data), "one commit per rule", "one commit per feature", 1)
		if tampered == string(data) {
			t.Fatal("tamper target not found in the archive")
		}
		os.WriteFile(archive, []byte(tampered), 0o644)
		cfg, err := config.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res := Run(root, cfg)
		if !hasFinding(res, "append_only", "memory/decisions.md", "append-only violation") {
			t.Errorf("want append_only/rewritten on the live log:\n%s", dump(res))
		}
		if res.Red() != 1 || len(infoFindings(res)) != 0 {
			t.Errorf("want exactly one RED and no INFO:\n%s", dump(res))
		}
	})
}
