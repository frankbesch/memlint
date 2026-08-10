package lint

import (
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

// The acceptance criterion: fixture-broken has exactly seven planted RED
// defects and two planted YELLOW ones. The count is the test -- if a rule
// starts over-reporting or silently stops reporting, this fails.
func TestFixtureBroken(t *testing.T) {
	res := runFixture(t, "fixture-broken")
	wantCounts(t, res, 7, 3)

	want := []struct{ rule, path, message string }{
		{"blocks", "AGENTS.md", "unterminated"},
		{"blocks", "docs/generated.md", "duplicate start marker"},
		{"mirrors", "CLAUDE.md", "mirrored files differ"},
		{"mirrors", "sync/left/a.md", "mirrored files differ"},
		{"mirrors", "sync/left/b.md", "missing from sync/right"},
		{"pointers", "memory/index.md", "dead reference: memory/missing.md"},
		{"pointers", "memory/index.md", "dead reference: docs/nope.md"},
		{"junk", "notes/scratch.tmp", `junk file matches "*.tmp"`},
		{"tokens", "memory/big.md", "estimated tokens exceeds budget"},
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
// The load-bearing one is memory/gone-anchored.md#section: that target exists
// nowhere and is referenced nowhere else, so if anchor-skipping regresses to
// strip-and-check, this file gains a sixth RED and TestFixtureBroken fails.
func TestFixtureBrokenSkipsAreNotReported(t *testing.T) {
	res := runFixture(t, "fixture-broken")
	for _, f := range res.Findings {
		for _, skipped := range []string{
			"reading/daily",
			"example.com",
			"reviews/",
			"gone-anchored",
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
