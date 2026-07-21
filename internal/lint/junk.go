package lint

import (
	"fmt"
	"io/fs"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleJunk = "junk"

// checkJunk reports files and directories anywhere under the root that match a
// configured glob. A matching directory is reported once and then pruned:
// listing every file inside a stray node_modules would bury the finding.
func checkJunk(r *runner, cfg *config.Junk) {
	r.walk(ruleJunk, func(rel string, d fs.DirEntry) error {
		if !d.IsDir() {
			r.mark(rel)
		}
		glob, matched := matchGlobs(cfg.Globs, rel, true)
		if !matched {
			return nil
		}
		kind := "file"
		if d.IsDir() {
			kind = "directory"
		}
		r.yellow(ruleJunk, rel, fmt.Sprintf("junk %s matches %q", kind, glob))
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
}
