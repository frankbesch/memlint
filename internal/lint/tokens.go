package lint

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"unicode/utf8"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleTokens = "tokens"

// checkTokens reports watched files whose estimated token count exceeds the
// budget, and watch globs that matched nothing at all.
//
// Unlike [junk], watch globs match the root-relative path only, never the
// basename: a budget is a statement about specific files, and matching
// "CLAUDE.md" against every nested CLAUDE.md would silently widen it.
//
// A glob that matches no file is YELLOW, not silence. A watch glob declares
// files its author expects to exist, so a stale one — a renamed directory, a
// typo — is a budget check that silently never runs, which is the same failure
// mode the config loader already refuses for unknown keys. [junk] is exempt:
// there, matching nothing is the desired state.
func checkTokens(r *runner, cfg *config.Tokens) {
	// Every glob is matched independently, rather than stopping at a file's
	// first match: a glob whose files all happen to match an earlier glob too
	// must still get credit for them.
	matched := make(map[string]bool, len(cfg.Watch))

	r.walk(ruleTokens, func(rel string, d fs.DirEntry) error {
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		any := false
		for _, g := range cfg.Watch {
			if ok, err := path.Match(g, rel); err == nil && ok {
				matched[g] = true
				any = true
			}
		}
		if !any {
			return nil
		}
		r.mark(rel)

		data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
		if err != nil {
			r.cannotVerify(ruleTokens, rel, err)
			return nil
		}
		if est := EstimateTokens(string(data)); est > cfg.Budget {
			r.yellow(ruleTokens, "tokens/over-budget", rel, fmt.Sprintf(
				"%d estimated tokens exceeds budget of %d", est, cfg.Budget))
		}
		return nil
	})

	for _, g := range cfg.Watch {
		if !matched[g] {
			r.add(Finding{
				Rule: ruleTokens, Code: "tokens/no-match", Severity: SeverityYellow, Path: g,
				Message: "watch glob matched no files",
				Detail:  "a stale watch glob is a budget check that silently never runs",
			})
		}
	}
}

// EstimateTokens approximates a token count as one token per four characters,
// rounded up. It counts runes, not bytes, so a document of multibyte
// characters is not overcounted several times over. This is an estimate and is
// reported as one -- memlint does not run a tokenizer.
func EstimateTokens(s string) int {
	return (utf8.RuneCountInString(s) + 3) / 4
}
