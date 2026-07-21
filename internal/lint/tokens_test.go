package lint

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func runTokens(root string, budget int, watch ...string) Result {
	return Run(root, &config.Config{Tokens: &config.Tokens{Watch: watch, Budget: budget}})
}

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"日本語です", 2}, // 5 runes, not 15 bytes
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// The budget is inclusive: a file exactly at budget passes, one character more
// warns. Getting this backwards would make every budget off by one file.
func TestTokensBudgetBoundary(t *testing.T) {
	const budget = 10

	t.Run("exactly at budget passes", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/a.md": strings.Repeat("x", budget*4),
		})
		wantCounts(t, runTokens(root, budget, "memory/*.md"), 0, 0)
	})

	t.Run("one character over warns", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/a.md": strings.Repeat("x", budget*4+1),
		})
		res := runTokens(root, budget, "memory/*.md")
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "11 estimated tokens exceeds budget of 10")
	})

	t.Run("multibyte content is counted in runes", func(t *testing.T) {
		// budget*4 multibyte runes is 120 bytes but only budget tokens.
		root := writeTree(t, map[string]string{
			"memory/a.md": strings.Repeat("あ", budget*4),
		})
		wantCounts(t, runTokens(root, budget, "memory/*.md"), 0, 0)
	})
}

func TestTokensWatchMatching(t *testing.T) {
	tree := map[string]string{
		"memory/big.md":      strings.Repeat("x", 400),
		"memory/deep/big.md": strings.Repeat("x", 400),
		"docs/big.md":        strings.Repeat("x", 400),
	}

	t.Run("only watched paths are measured", func(t *testing.T) {
		wantCounts(t, runTokens(writeTree(t, tree), 10, "memory/*.md"), 0, 1)
	})

	// Unlike the junk rule, watch globs never fall back to basename matching: a
	// budget names specific files, and matching every nested big.md would widen
	// it silently.
	t.Run("basenames are not matched", func(t *testing.T) {
		wantCounts(t, runTokens(writeTree(t, tree), 10, "big.md"), 0, 0)
	})

	t.Run("overlapping globs produce one finding per file", func(t *testing.T) {
		root := writeTree(t, map[string]string{"memory/big.md": strings.Repeat("x", 400)})
		wantCounts(t, runTokens(root, 10, "memory/*.md", "memory/big.md"), 0, 1)
	})
}
