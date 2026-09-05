package lint

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleIDs = "ids"

// checkIDs verifies that every id opening a line is unique across all listed
// files. Source: two FBOS sessions each wrote D-102 on the same day; the
// allocator upstream (next-id.sh) prevents new collisions but cannot see the
// file, and nothing else did.
//
// What counts as an id: the configured pattern, matched per line, and only
// when the match begins at column 1 — a mid-line "see D-001" is a citation,
// not an id, whatever the pattern says. The id itself is the first capture
// group, or the whole match without one. Gaps are not findings: D-070 may be
// legitimately absent.
//
// "First" is the earliest occurrence in source order — literals in config
// order, then glob matches in walk order — then by line. Every later
// occurrence is its own RED, each citing that first one.
func checkIDs(r *runner, cfg *config.IDs) {
	// Validated at config load; a compile failure here is a programming error.
	re := regexp.MustCompile(cfg.EffectivePattern())

	type at struct {
		path string
		line int
	}
	first := map[string]at{}
	known := make(map[string]bool, len(cfg.Known))
	for _, k := range cfg.Known {
		known[k] = false // false until a collision is actually seen
	}

	sources := expandSources(r, ruleIDs, cfg.Files, Finding{
		Rule: ruleIDs, Code: "ids/no-match", Severity: SeverityYellow,
		Message: "files glob matched no files",
		Detail:  "a stale files glob is id coverage that silently never runs",
	})
	for _, rel := range sources {
		abs, ok := r.resolve(rel)
		if !ok {
			r.red(ruleIDs, "ids/escape", rel, "id source escapes the repository root")
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				r.red(ruleIDs, "ids/missing-source", rel, "id source file does not exist")
			} else {
				r.cannotVerify(ruleIDs, rel, err)
			}
			continue
		}
		r.mark(rel)

		for i, line := range strings.Split(string(data), "\n") {
			id, ok := lineID(re, line)
			if !ok {
				continue
			}
			if prev, dup := first[id]; dup {
				if _, listed := known[id]; listed {
					known[id] = true
					r.add(Finding{
						Rule: ruleIDs, Code: "ids/known-duplicate", Severity: SeverityInfo,
						Path: rel, Line: i + 1, RelatedPath: prev.path,
						Message: fmt.Sprintf("known duplicate id %s: first at %s:%d", id, prev.path, prev.line),
					})
					continue
				}
				r.add(Finding{
					Rule: ruleIDs, Code: "ids/duplicate", Severity: SeverityRed,
					Path: rel, Line: i + 1, RelatedPath: prev.path,
					Message: fmt.Sprintf("duplicate id %s: first at %s:%d", id, prev.path, prev.line),
				})
				continue
			}
			first[id] = at{rel, i + 1}
		}
	}

	for _, k := range cfg.Known {
		if !known[k] {
			r.add(Finding{
				Rule: ruleIDs, Code: "ids/known-unused", Severity: SeverityYellow, Path: k,
				Message: "known id never collides: stale allowlist entry",
				Detail:  "an entry in known that has no duplicate would silently excuse a future one",
			})
		}
	}
}

// lineID extracts the id opening line, if any. The match must start at
// column 1; the pattern's own anchoring, if it has one, is redundant with
// that rule rather than a substitute for it.
func lineID(re *regexp.Regexp, line string) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	loc := re.FindStringSubmatchIndex(line)
	if loc == nil || loc[0] != 0 {
		return "", false
	}
	if len(loc) >= 4 && loc[2] >= 0 {
		return line[loc[2]:loc[3]], true
	}
	return line[loc[0]:loc[1]], true
}
