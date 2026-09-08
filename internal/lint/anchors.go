package lint

import (
	"regexp"
	"strconv"
	"strings"
)

// Anchor resolution for [pointers] (pointers/dead-anchor, v0.9). An anchored
// reference "file.md#anchor" resolves when the target markdown carries a
// heading whose GitHub-style slug is the anchor, or an explicit HTML anchor
// with that id or name. Only markdown targets are checked; the rule never
// reads other file types.

var (
	headingRe     = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
	fenceRe       = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})")
	htmlAnchorRe  = regexp.MustCompile(`<a\s+[^>]*?(?:id|name)\s*=\s*["']([^"']+)["']`)
	customIDRe    = regexp.MustCompile(`\s*\{#([A-Za-z0-9_-]+)\}\s*$`)
	mdLinkRe      = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	inlineCodeRe  = regexp.MustCompile("`([^`]*)`")
	emphasisRe    = regexp.MustCompile(`[*~]+`)
	closingHashRe = regexp.MustCompile(`\s+#+\s*$`)
	slugDropRe    = regexp.MustCompile(`[^\p{L}\p{N} _-]`)
)

// markdownAnchors returns the set of anchors a markdown document exposes:
// every heading's slug (duplicates suffixed -1, -2, … as GitHub does), any
// `{#custom-id}` heading suffix, and any `<a id=…>` / `<a name=…>`.
// Headings inside fenced code blocks are not headings.
func markdownAnchors(src string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	inFence, fenceMark := false, ""
	for _, line := range strings.Split(src, "\n") {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			mark := m[1]
			if !inFence {
				inFence, fenceMark = true, mark
			} else if strings.HasPrefix(mark, fenceMark[:1]) && len(mark) >= len(fenceMark) {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		for _, m := range htmlAnchorRe.FindAllStringSubmatch(line, -1) {
			anchors[m[1]] = true
		}
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := m[2]
		if c := customIDRe.FindStringSubmatch(text); c != nil {
			anchors[c[1]] = true
			text = customIDRe.ReplaceAllString(text, "")
		}
		slug := headingSlug(text)
		if n, dup := seen[slug]; dup {
			seen[slug] = n + 1
			anchors[slug+"-"+strconv.Itoa(n+1)] = true
		} else {
			seen[slug] = 0
			anchors[slug] = true
		}
	}
	return anchors
}

// headingSlug applies GitHub's heading-anchor rule: strip inline markup (link
// and code delimiters, * and ~ emphasis — underscores stay, as GitHub keeps
// them), drop a closing ### sequence, lowercase, drop everything but letters,
// numbers, spaces, hyphens and underscores, then spaces to hyphens. Leading and trailing hyphens survive
// exactly as GitHub keeps them.
func headingSlug(text string) string {
	text = mdLinkRe.ReplaceAllString(text, "$1")
	text = inlineCodeRe.ReplaceAllString(text, "$1")
	text = emphasisRe.ReplaceAllString(text, "")
	text = closingHashRe.ReplaceAllString(text, "")
	text = strings.ToLower(strings.TrimSpace(text))
	text = slugDropRe.ReplaceAllString(text, "")
	return strings.ReplaceAll(text, " ", "-")
}

// isMarkdownTarget reports whether an anchor on target is checkable.
func isMarkdownTarget(target string) bool {
	t := strings.ToLower(target)
	return strings.HasSuffix(t, ".md") || strings.HasSuffix(t, ".markdown")
}
