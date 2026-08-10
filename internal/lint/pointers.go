package lint

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const rulePointers = "pointers"

// Ref is a repo-path-like reference extracted from a document.
type Ref struct {
	// Raw is the reference exactly as written, after surrounding markdown
	// punctuation is trimmed.
	Raw string
	// Target is Raw normalized to a cleaned, slash-separated path.
	Target string
	// Line is the 1-based line of the reference's first occurrence.
	Line int
}

var (
	// backtickRe matches inline code spans: `memory/foo.md`.
	backtickRe = regexp.MustCompile("`([^`\n]+)`")
	// linkRe matches markdown link and image destinations: [t](dest), ![t](dest).
	linkRe = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\s]*)\)`)
)

const (
	// trimLeading and trimTrailing strip the markdown punctuation that tends to
	// wrap a path in prose. Glob metacharacters are deliberately absent: a
	// trailing * must survive trimming so the reject list below can catch it.
	trimLeading  = "`\"'“”‘’([{"
	trimTrailing = "`\"'“”‘’)]}.,;:!"

	// rejectChars marks a candidate as not-a-path. It covers the spec's skips
	// (< > for placeholders, # for anchors) plus markdown syntax that survived
	// tokenization and glob metacharacters, which are placeholders for a set of
	// files rather than a reference to one.
	rejectChars = "[]()`\"'*?<>#|"
)

// ExtractRefs pulls repo-path-like references out of a document, deduplicated
// by target, ordered by first appearance.
//
// Three passes feed it: inline code spans, markdown link and image
// destinations, and bare whitespace-delimited tokens. A candidate survives only
// if it contains a slash and none of the skip markers -- URLs (://), the YYYY
// date placeholder, angle-bracket placeholders, anchors (#), glob
// metacharacters, or anything escaping the repository root.
func ExtractRefs(src string) []Ref {
	var refs []Ref
	seen := map[string]bool{}

	for i, line := range strings.Split(src, "\n") {
		for _, cand := range lineCandidates(line) {
			raw, target, ok := normalizeCandidate(cand)
			if !ok || seen[target] {
				continue
			}
			seen[target] = true
			refs = append(refs, Ref{Raw: raw, Target: target, Line: i + 1})
		}
	}
	return refs
}

// CheckableRefs returns the subset of ExtractRefs whose first path segment is
// one of roots. That single filter is what separates a reference memlint is
// responsible for from one it must leave alone: an unlisted root means the
// reference points somewhere memlint was never told about.
func CheckableRefs(src string, roots []string) []Ref {
	rootSet := setOf(roots)
	var out []Ref
	for _, ref := range ExtractRefs(src) {
		if _, ok := rootSet[firstSegment(ref.Target)]; ok {
			out = append(out, ref)
		}
	}
	return out
}

func firstSegment(target string) string {
	if i := strings.Index(target, "/"); i >= 0 {
		return target[:i]
	}
	return target
}

type candidate struct {
	off  int
	text string
}

// lineCandidates gathers every candidate substring of a line, ordered by the
// position it was found at so that output order is stable.
func lineCandidates(line string) []candidate {
	var cands []candidate
	// spans records the full extent of each code span and link, so the bare
	// token pass does not shred them. Without this, `memory/two words.md`
	// yields the fragment "memory/two", which would be reported as a dead
	// reference to a file nobody ever wrote down.
	var spans [][2]int

	for _, re := range []*regexp.Regexp{backtickRe, linkRe} {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			cands = append(cands, candidate{off: m[2], text: line[m[2]:m[3]]})
			spans = append(spans, [2]int{m[0], m[1]})
		}
	}
	for _, tok := range bareTokens(line) {
		if !overlapsAny(tok, spans) {
			cands = append(cands, tok)
		}
	}

	// Stable sort by offset: a path found by two passes keeps one position.
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && cands[j].off < cands[j-1].off; j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}
	return cands
}

// overlapsAny reports whether a token intersects one of the given spans.
func overlapsAny(tok candidate, spans [][2]int) bool {
	start, end := tok.off, tok.off+len(tok.text)
	for _, s := range spans {
		if start < s[1] && s[0] < end {
			return true
		}
	}
	return false
}

// bareTokens splits a line on whitespace, keeping each token's offset.
func bareTokens(line string) []candidate {
	var out []candidate
	start := -1
	for i := 0; i <= len(line); i++ {
		if i == len(line) || isSpaceByte(line[i]) {
			if start >= 0 {
				out = append(out, candidate{off: start, text: line[start:i]})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\v' || b == '\f'
}

// normalizeCandidate trims markdown punctuation and applies every skip rule,
// returning the trimmed reference and its cleaned target.
func normalizeCandidate(c candidate) (raw, target string, ok bool) {
	raw = strings.TrimRight(strings.TrimLeft(c.text, trimLeading), trimTrailing)
	if raw == "" || !strings.Contains(raw, "/") {
		return "", "", false
	}
	if strings.Contains(raw, "://") {
		return "", "", false
	}
	if strings.ContainsAny(raw, rejectChars) {
		return "", "", false
	}
	if strings.Contains(raw, "YYYY") {
		return "", "", false
	}
	if strings.IndexFunc(raw, func(r rune) bool { return r == ' ' || r == '\t' }) >= 0 {
		return "", "", false
	}
	if strings.HasPrefix(raw, "/") {
		return "", "", false
	}
	target = config.CleanRel(raw)
	if target == "." || target == ".." || strings.HasPrefix(target, "../") {
		return "", "", false
	}
	return raw, target, true
}

// checkPointers verifies that every checkable reference in the configured
// files resolves to something that exists.
func checkPointers(r *runner, cfg *config.Pointers) {
	for _, f := range cfg.Files {
		rel := config.CleanRel(f)
		abs, ok := r.resolve(rel)
		if !ok {
			r.red(rulePointers, "pointers/escape", rel, "pointer source escapes the repository root")
			continue
		}
		// Read the whole file rather than scanning it: bufio.Scanner's default
		// 64 KiB line cap would silently truncate a long line, and a reference
		// dropped by the reader is indistinguishable from one that is fine.
		data, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				r.red(rulePointers, "pointers/missing-source", rel, "pointer source file does not exist")
			} else {
				r.cannotVerify(rulePointers, rel, err)
			}
			continue
		}
		r.mark(rel)

		for _, ref := range CheckableRefs(string(data), cfg.Roots) {
			targetAbs, ok := r.resolve(ref.Target)
			if !ok {
				continue
			}
			if _, err := os.Stat(targetAbs); err != nil {
				if os.IsNotExist(err) {
					r.add(Finding{
						Rule: rulePointers, Code: "pointers/dead-ref", Severity: SeverityRed, Path: rel,
						RelatedPath: ref.Target, Line: ref.Line,
						Message: fmt.Sprintf("dead reference: %s does not exist", ref.Target),
					})
				} else {
					r.cannotVerify(rulePointers, rel, err)
				}
			}
		}
	}
}
