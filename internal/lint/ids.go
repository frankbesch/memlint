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

	first := map[string]idAt{}
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
		data, ok := readIDSource(r, rel)
		if !ok {
			continue
		}

		var prevID string
		prevLine := 0
		for i, line := range strings.Split(string(data), "\n") {
			id, ok := lineID(re, line)
			if !ok {
				continue
			}
			if cfg.Ordered && prevID != "" && idLess(id, prevID) {
				r.add(Finding{
					Rule: ruleIDs, Code: "ids/out-of-order", Severity: SeverityRed,
					Path: rel, Line: i + 1,
					Message: fmt.Sprintf("id %s follows %s (line %d)", id, prevID, prevLine),
				})
			}
			prevID, prevLine = id, i+1
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
			first[id] = idAt{rel, i + 1}
		}
	}

	if len(cfg.CitedIn) > 0 {
		checkCites(r, cfg, first)
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

// idAt is where an id was first seen.
type idAt struct {
	path string
	line int
}

// readIDSource reads one resolved source, reporting the usual shapes.
func readIDSource(r *runner, rel string) ([]byte, bool) {
	abs, ok := r.resolve(rel)
	if !ok {
		r.red(ruleIDs, "ids/escape", rel, "id source escapes the repository root")
		return nil, false
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			r.red(ruleIDs, "ids/missing-source", rel, "id source file does not exist")
		} else {
			r.cannotVerify(ruleIDs, rel, err)
		}
		return nil, false
	}
	r.mark(rel)
	return data, true
}

// checkCites verifies that every id cited in the cited_in files is an entry.
// Dead citations are the id-space version of dead pointers: a ruling that
// was renumbered or never written, still referenced as if it existed.
func checkCites(r *runner, cfg *config.IDs, entries map[string]idAt) {
	re := regexp.MustCompile(cfg.EffectiveCitePattern())
	sources := expandSources(r, ruleIDs, cfg.CitedIn, Finding{
		Rule: ruleIDs, Code: "ids/no-match", Severity: SeverityYellow,
		Message: "cited_in glob matched no files",
		Detail:  "a stale cited_in glob is citation coverage that silently never runs",
	})
	for _, rel := range sources {
		data, ok := readIDSource(r, rel)
		if !ok {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			seen := map[string]bool{}
			for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
				id := line[m[0]:m[1]]
				if len(m) >= 4 && m[2] >= 0 {
					id = line[m[2]:m[3]]
				}
				if seen[id] {
					continue
				}
				seen[id] = true
				if _, ok := entries[id]; ok {
					continue
				}
				r.add(Finding{
					Rule: ruleIDs, Code: "ids/dead-cite", Severity: SeverityRed,
					Path: rel, Line: i + 1,
					Message: fmt.Sprintf("cited id %s has no entry", id),
				})
			}
		}
	}
}

// idLess orders ids by the number they carry (D-002 < D-010), falling back
// to string order when either has none.
func idLess(a, b string) bool {
	na, oka := idNumber(a)
	nb, okb := idNumber(b)
	if oka && okb {
		return na < nb
	}
	return a < b
}

func idNumber(id string) (int, bool) {
	n, ok := 0, false
	for _, c := range id {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			ok = true
		} else if ok {
			break
		}
	}
	return n, ok
}
