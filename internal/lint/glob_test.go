package lint

import (
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func TestGlobMatchRecursive(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"memory/*.md", "memory/a.md", true},
		{"memory/*.md", "memory/sub/a.md", false},
		{"memory/**/*.md", "memory/a.md", true},
		{"memory/**/*.md", "memory/sub/a.md", true},
		{"memory/**/*.md", "memory/sub/deep/a.md", true},
		{"memory/**/*.md", "notes/a.md", false},
		{"**/*.md", "a.md", true},
		{"**/*.md", "x/y/a.md", true},
		{"**/*.tmp", "x/y/a.md", false},
		{"memory/**", "memory/a.md", true},
		{"memory/**", "memory/sub/a.md", true},
		{"memory/**", "memory", false},
		{"**", "anything/at/all", true},
		{"docs/?.md", "docs/a.md", true},
		{"docs/[ab].md", "docs/b.md", true},
		{"docs/[ab].md", "docs/c.md", false},
		{"a.b", "aXb", false}, // dots are literal
	}
	for _, tc := range cases {
		if got := globMatch(tc.pattern, tc.rel); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.rel, got, tc.want)
		}
	}
}

// ** reaches nested trees in every glob-taking rule.
func TestRecursiveGlobsAcrossRules(t *testing.T) {
	root := writeTree(t, map[string]string{
		"memory/a.md":             "D-001 | a\n",
		"memory/deep/er/b.md":     "D-001 | dup\n" + "x" + "\n",
		"memory/deep/scratch.tmp": "",
	})
	res := Run(root, &config.Config{
		Tokens: &config.Tokens{Watch: []string{"memory/**/*.md"}, Budget: 3},
		IDs:    &config.IDs{Files: []string{"memory/**/*.md"}},
		Junk:   &config.Junk{Globs: []string{"memory/**/*.tmp"}},
	})
	for _, w := range []struct{ rule, path, msg string }{
		{"tokens", "memory/deep/er/b.md", "exceeds budget"},
		{"ids", "memory/deep/er/b.md", "duplicate id D-001"},
		{"junk", "memory/deep/scratch.tmp", "junk file"},
	} {
		if !hasFinding(res, w.rule, w.path, w.msg) {
			t.Errorf("missing %s %s %q:\n%s", w.rule, w.path, w.msg, dump(res))
		}
	}
	wantCounts(t, res, 1, 2)
}
