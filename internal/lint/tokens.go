package lint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleTokens = "tokens"

// checkTokens reports watched files whose estimated token count exceeds the
// budget.
//
// Unlike [junk], watch globs match the root-relative path only, never the
// basename: a budget is a statement about specific files, and matching
// "CLAUDE.md" against every nested CLAUDE.md would silently widen it.
func checkTokens(r *runner, cfg *config.Tokens) {
	r.walk(ruleTokens, func(rel string, d fs.DirEntry) error {
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if _, matched := matchGlobs(cfg.Watch, rel, false); !matched {
			return nil
		}
		r.mark(rel)

		data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
		if err != nil {
			r.cannotVerify(ruleTokens, rel, err)
			return nil
		}
		if est := EstimateTokens(string(data)); est > cfg.Budget {
			r.yellow(ruleTokens, rel, fmt.Sprintf(
				"%d estimated tokens exceeds budget of %d", est, cfg.Budget))
		}
		return nil
	})
}

// EstimateTokens approximates a token count as one token per four characters,
// rounded up. It counts runes, not bytes, so a document of multibyte
// characters is not overcounted several times over. This is an estimate and is
// reported as one -- memlint does not run a tokenizer.
func EstimateTokens(s string) int {
	return (utf8.RuneCountInString(s) + 3) / 4
}
