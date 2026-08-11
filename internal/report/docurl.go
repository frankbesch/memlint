package report

import "strings"

// DocBase is the canonical home of the finding-code documentation. Pinned to
// main rather than a tag: the docs page keeps entries for every code that ever
// shipped, so main is always at least as complete as the binary linking to it.
const DocBase = "https://github.com/frankbesch/memlint/blob/main/docs/findings.md"

// DocURL maps a finding code to its documentation anchor. GitHub's heading
// anchors keep letters, digits, underscores, and dashes and strip the slash,
// so "pointers/dead-ref" becomes "#pointersdead-ref". The <rule>/unverifiable
// family shares one section: every spelling means the same thing.
func DocURL(code string) string {
	if code == "" {
		return ""
	}
	if strings.HasSuffix(code, "/unverifiable") {
		return DocBase + "#unverifiable"
	}
	return DocBase + "#" + strings.ReplaceAll(code, "/", "")
}
