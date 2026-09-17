package lint

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/frankbesch/memvet/internal/config"
)

const rulePointers = "pointers"

// Ref is a repo-path-like reference extracted from a document.
type Ref struct {
	// Raw is the reference exactly as written, after surrounding markdown
	// punctuation is trimmed. For anchored references it keeps the #anchor.
	Raw string
	// Target is the base path — Raw with any anchor removed — normalized to a
	// cleaned, slash-separated form. Existence checks run against Target, so a
	// bare ref and an anchored ref to the same file deduplicate to one.
	Target string
	// Anchor is the fragment after "#", without the "#". Empty for bare refs.
	// Since v0.9 an anchor on a markdown target must name a heading slug or
	// explicit HTML anchor in that file (pointers/dead-anchor).
	Anchor string
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

	// rejectChars marks a candidate's base path as not-a-path. It covers the
	// spec's skips (< > for placeholders) plus markdown syntax that survived
	// tokenization and glob metacharacters, which are placeholders for a set of
	// files rather than a reference to one. "#" is absent since v0.6: a single
	// anchor splits off before this check, and more than one is rejected
	// explicitly.
	rejectChars = "[]()`\"'*?<>|"
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
			ref, ok := normalizeCandidate(cand)
			if !ok || seen[ref.Target] {
				continue
			}
			seen[ref.Target] = true
			ref.Line = i + 1
			refs = append(refs, ref)
		}
	}
	return refs
}

// ExtractAnchoredRefs is ExtractRefs deduplicated by target+anchor instead of
// target, and limited to anchored references: the input to the dead-anchor
// check, where two different anchors on one file are two different claims.
func ExtractAnchoredRefs(src string) []Ref {
	var refs []Ref
	seen := map[string]bool{}
	for i, line := range strings.Split(src, "\n") {
		for _, cand := range lineCandidates(line) {
			ref, ok := normalizeCandidate(cand)
			if !ok || ref.Anchor == "" {
				continue
			}
			key := ref.Target + "#" + ref.Anchor
			if seen[key] {
				continue
			}
			seen[key] = true
			ref.Line = i + 1
			refs = append(refs, ref)
		}
	}
	return refs
}

// checkableAnchoredRefs filters ExtractAnchoredRefs by roots, like CheckableRefs.
func checkableAnchoredRefs(src string, roots []string) []Ref {
	rootSet := setOf(roots)
	var out []Ref
	for _, ref := range ExtractAnchoredRefs(src) {
		if _, ok := rootSet[firstSegment(ref.Target)]; ok {
			out = append(out, ref)
		}
	}
	return out
}

// CheckableRefs returns the subset of ExtractRefs whose first path segment is
// one of roots. That single filter is what separates a reference memvet is
// responsible for from one it must leave alone: an unlisted root means the
// reference points somewhere memvet was never told about.
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
// returning the reference with its cleaned base-path target.
//
// A single "#" splits the candidate into base + anchor, and every filter below
// then applies to the base: an anchored reference to a real file is a claim
// about that file, and the base is what can be checked today. More than one
// "#" is not a path+anchor and is rejected outright, as before v0.6.
func normalizeCandidate(c candidate) (ref Ref, ok bool) {
	raw := strings.TrimRight(strings.TrimLeft(c.text, trimLeading), trimTrailing)
	base, anchor := raw, ""
	if i := strings.Index(raw, "#"); i >= 0 {
		if strings.Count(raw, "#") > 1 {
			return Ref{}, false
		}
		base, anchor = raw[:i], raw[i+1:]
	}
	if base == "" || !strings.Contains(base, "/") {
		return Ref{}, false
	}
	if strings.Contains(base, "://") {
		return Ref{}, false
	}
	if strings.ContainsAny(base, rejectChars) {
		return Ref{}, false
	}
	if strings.Contains(base, "YYYY") {
		return Ref{}, false
	}
	if strings.IndexFunc(base, func(r rune) bool { return r == ' ' || r == '\t' }) >= 0 {
		return Ref{}, false
	}
	if strings.HasPrefix(base, "/") {
		return Ref{}, false
	}
	target := config.CleanRel(base)
	if target == "." || target == ".." || strings.HasPrefix(target, "../") {
		return Ref{}, false
	}
	return Ref{Raw: raw, Target: target, Anchor: anchor}, true
}

// SiblingRoot is the [pointers] roots entry that turns on sibling checking:
// a markdown link or image destination with no slash resolves against the
// source file's own directory (v0.11). It exists for flat memory folders —
// Claude Code's auto-memory is MEMORY.md beside its notes, indexed as
// `[Title](note.md)` — where no first-segment root could ever match.
const SiblingRoot = "."

// hasSiblingRoot reports whether roots turns on the sibling pass.
func hasSiblingRoot(roots []string) bool {
	for _, r := range roots {
		if r == SiblingRoot {
			return true
		}
	}
	return false
}

// siblingRefs extracts link and image destinations that carry no slash and
// rewrites each target relative to dir, the source file's directory. Only
// the link pass feeds it: a bare token like "notes.md" in prose is a word,
// not a claim, and flagging it would make the rule unusable on real notes.
// Dedup is by target when anchored is false and by target+anchor otherwise,
// mirroring ExtractRefs and ExtractAnchoredRefs.
func siblingRefs(src, dir string, anchored bool) []Ref {
	var refs []Ref
	seen := map[string]bool{}
	for i, line := range strings.Split(src, "\n") {
		for _, m := range linkRe.FindAllStringSubmatchIndex(line, -1) {
			ref, ok := normalizeSibling(line[m[2]:m[3]])
			if !ok || (anchored && ref.Anchor == "") {
				continue
			}
			ref.Target = config.CleanRel(path.Join(dir, ref.Target))
			key := ref.Target
			if anchored {
				key += "#" + ref.Anchor
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			ref.Line = i + 1
			refs = append(refs, ref)
		}
	}
	return refs
}

// normalizeSibling applies the skip rules to a link destination and accepts
// it only when the base has no slash: a destination with one is the
// ordinary pass's business, gated by the named roots.
func normalizeSibling(dest string) (Ref, bool) {
	raw := strings.TrimSpace(dest)
	base, anchor := raw, ""
	if i := strings.Index(raw, "#"); i >= 0 {
		if strings.Count(raw, "#") > 1 {
			return Ref{}, false
		}
		base, anchor = raw[:i], raw[i+1:]
	}
	base = strings.TrimPrefix(base, "./")
	if base == "" || strings.Contains(base, "/") || strings.Contains(base, ":") {
		return Ref{}, false
	}
	if strings.ContainsAny(base, rejectChars) || strings.Contains(base, "YYYY") {
		return Ref{}, false
	}
	if base == "." || base == ".." {
		return Ref{}, false
	}
	return Ref{Raw: raw, Target: base, Anchor: anchor}, true
}

// checkPointers verifies that every checkable reference in the configured
// files resolves to something that exists.
//
// A files entry with glob metacharacters is expanded against root-relative
// paths only, never basenames — a source list is a statement about specific
// files, and basename matching would silently widen it. A glob matching
// nothing is YELLOW for the same reason a stale [tokens] watch glob is:
// declared coverage that silently never runs.
func checkPointers(r *runner, cfg *config.Pointers) {
	for _, rel := range pointerSources(r, cfg.Files) {
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

		exists := map[string]bool{}
		refs := CheckableRefs(string(data), cfg.Roots)
		anchored := checkableAnchoredRefs(string(data), cfg.Roots)
		if hasSiblingRoot(cfg.Roots) {
			dir := path.Dir(rel)
			refs = append(refs, siblingRefs(string(data), dir, false)...)
			anchored = append(anchored, siblingRefs(string(data), dir, true)...)
		}
		for _, ref := range refs {
			targetAbs, ok := r.resolve(ref.Target)
			if !ok {
				continue
			}
			if _, err := os.Stat(targetAbs); err == nil {
				exists[ref.Target] = true
			} else {
				if os.IsNotExist(err) {
					msg := fmt.Sprintf("dead reference: %s does not exist", ref.Target)
					if ref.Anchor != "" {
						msg += fmt.Sprintf(" (referenced as %s)", ref.Raw)
					}
					r.add(Finding{
						Rule: rulePointers, Code: "pointers/dead-ref", Severity: SeverityRed, Path: rel,
						RelatedPath: ref.Target, Line: ref.Line,
						Message: msg,
					})
				} else {
					r.cannotVerify(rulePointers, rel, err)
				}
			}
		}
		checkAnchors(r, rel, anchored, exists)
	}
}

// checkAnchors runs the dead-anchor check for one source file: every anchored
// reference whose base exists and is markdown must name an anchor that
// markdown exposes. A dead base is already pointers/dead-ref and is not
// reported twice. Target files are read once each.
func checkAnchors(r *runner, rel string, anchored []Ref, exists map[string]bool) {
	cache := map[string]map[string]bool{}
	for _, ref := range anchored {
		if !exists[ref.Target] || !isMarkdownTarget(ref.Target) {
			continue
		}
		anchors, ok := cache[ref.Target]
		if !ok {
			abs, resolved := r.resolve(ref.Target)
			if !resolved {
				continue
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				r.cannotVerify(rulePointers, rel, err)
				continue
			}
			anchors = markdownAnchors(string(data))
			cache[ref.Target] = anchors
		}
		if anchors[ref.Anchor] {
			continue
		}
		r.add(Finding{
			Rule: rulePointers, Code: "pointers/dead-anchor", Severity: SeverityRed, Path: rel,
			RelatedPath: ref.Target, Line: ref.Line,
			Message: fmt.Sprintf("dead anchor: %s has no heading or anchor %q", ref.Target, ref.Anchor),
			Detail:  anchorHint(anchors),
		})
	}
}

// anchorHint lists the anchors the target does expose, so the fix is one
// glance away. Capped so a long document does not flood the report.
func anchorHint(anchors map[string]bool) string {
	keys := sortedKeys(anchors)
	if len(keys) == 0 {
		return "the target has no headings"
	}
	const cap_ = 8
	more := ""
	if len(keys) > cap_ {
		more = fmt.Sprintf(" (+%d more)", len(keys)-cap_)
		keys = keys[:cap_]
	}
	return "anchors present: #" + strings.Join(keys, " #") + more
}

// pointerSources resolves the configured files list — literals kept as-is,
// globs expanded by a walk — into a deduplicated list of source paths.
// Zero-match globs are reported here; a missing literal is reported by the
// caller's read, so that the two failure shapes keep their severities:
// a named file that is gone is RED, a pattern that matches nothing is YELLOW.
func pointerSources(r *runner, files []string) []string {
	return expandSources(r, rulePointers, files, Finding{
		Rule: rulePointers, Code: "pointers/no-match", Severity: SeverityYellow,
		Message: "files glob matched no files",
		Detail:  "a stale source glob is pointer coverage that silently never runs",
	})
}

// expandSources is the shared files resolver behind [pointers] and [ids]:
// literal entries first, in config order, then glob matches in walk order,
// deduplicated. Globs match the root-relative path only, never the basename.
// noMatch is the finding template emitted, with Path set to the glob, for
// each glob that matched nothing.
func expandSources(r *runner, rule string, files []string, noMatch Finding) []string {
	var sources []string
	seen := map[string]bool{}
	var globs []string
	for _, f := range files {
		if config.IsGlob(f) {
			globs = append(globs, f)
			continue
		}
		rel := config.CleanRel(f)
		if !seen[rel] {
			seen[rel] = true
			sources = append(sources, rel)
		}
	}
	if len(globs) == 0 {
		return sources
	}

	matched := make(map[string]bool, len(globs))
	r.walk(rule, func(rel string, d fs.DirEntry) error {
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		for _, g := range globs {
			if globMatch(g, rel) {
				matched[g] = true
				if !seen[rel] {
					seen[rel] = true
					sources = append(sources, rel)
				}
			}
		}
		return nil
	})
	for _, g := range globs {
		if !matched[g] {
			f := noMatch
			f.Path = g
			r.add(f)
		}
	}
	return sources
}
