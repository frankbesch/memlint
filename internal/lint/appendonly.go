package lint

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleAppendOnly = "append_only"

// maxLineEcho caps how much of a divergent line is echoed back, so a minified
// or single-line file cannot flood the report.
const maxLineEcho = 120

// checkAppendOnly verifies that each configured file still begins with the
// content stored at the baseline ref -- HEAD by default, or --base <ref>.
//
// Default scope, deliberately narrow: comparing HEAD against the working tree
// is a working-tree rewrite guard, not historical immutability enforcement --
// a rewrite that has already been committed becomes the new baseline and
// passes (and in a CI checkout the working tree IS HEAD, so the default can
// never fire there). Passing --base with the pull request's base branch is
// what turns this into a PR gate: a rewrite committed inside the PR diverges
// from the base blob and is caught.
//
// Two v0.7 refinements, both off unless the config asks for them or the
// repository is mid-rotation:
//
//   - header_lines = N exempts the first N lines of every listed file. That is
//     the ONLY mutable span; a live log's pointer header changes at each
//     rotation and must not read as a rewrite.
//   - The rotation allowance. When a file no longer begins with its baseline,
//     the cut span (baseline lines that are gone from the working copy, header
//     excluded) is searched for, verbatim and whole-line, in every OTHER listed
//     file that has no baseline of its own at the ref — untracked, or created
//     after --base. Found: INFO append_only/rotated instead of RED, and the
//     destination's no-baseline YELLOW is withdrawn, since the moved span is
//     its baseline. Not found: the RED, unchanged.
func checkAppendOnly(r *runner, cfg *config.AppendOnly) {
	ref := cfg.BaseRef
	if ref == "" {
		ref = "HEAD"
	}

	if _, err := exec.LookPath("git"); err != nil {
		for _, f := range cfg.Files {
			r.add(Finding{
				Rule: ruleAppendOnly, Code: "append_only/no-baseline", Severity: SeverityYellow, Path: config.CleanRel(f),
				Message: "no git baseline: append-only not verified",
				Detail:  "git executable not found in PATH",
			})
		}
		return
	}

	// Pass 1: establish every file's baseline first, because a rewrite in one
	// file is judged against the files that have none.
	var checked, fresh []*appendOnlyFile
	for _, f := range cfg.Files {
		rel := config.CleanRel(f)
		abs, ok := r.resolve(rel)
		if !ok {
			r.red(ruleAppendOnly, "append_only/escape", rel, "path escapes the repository root")
			continue
		}
		e := &appendOnlyFile{rel: rel}
		e.baseline, e.baselineErr = gitShowRef(r.root, ref, rel)
		e.current, e.readErr = os.ReadFile(abs)
		if e.baselineErr != nil {
			// No baseline is not a violation: the invariant could not be
			// established (spec: YELLOW). Reported in pass 3, unless a rotation
			// lands here first. An unreadable candidate simply never matches.
			fresh = append(fresh, e)
			continue
		}
		if e.readErr != nil {
			if os.IsNotExist(e.readErr) {
				r.red(ruleAppendOnly, "append_only/missing", rel, fmt.Sprintf(
					"file exists at %s but is missing from the working tree", ref))
			} else {
				r.cannotVerify(ruleAppendOnly, rel, e.readErr)
			}
			continue
		}
		r.mark(rel)
		checked = append(checked, e)
	}

	// Pass 2: the prefix check, header excluded, with the rotation allowance.
	rotatedInto := map[string]bool{}
	for _, e := range checked {
		n := cfg.HeaderFor(e.rel)
		base := stripHeader(e.baseline, n)
		cur := stripHeader(e.current, n)
		if hasAppendOnlyPrefix(cur, base) {
			continue
		}
		if rot, ok := findRotation(base, cur, fresh, cfg.HeaderFor); ok {
			rotatedInto[rot.dst] = true
			r.mark(rot.dst)
			r.add(Finding{
				Rule: ruleAppendOnly, Code: "append_only/rotated", Severity: SeverityInfo,
				Path: e.rel, RelatedPath: rot.dst, Line: rot.firstLine + n,
				Message: fmt.Sprintf("rotated → %s (%d lines moved verbatim)", rot.dst, rot.lines),
				Detail: fmt.Sprintf("baseline lines %d-%d left %s and appear unchanged in %s",
					rot.firstLine+n, rot.firstLine+n+rot.lines-1, e.rel, rot.dst),
			})
			continue
		}
		line, was, now := firstDivergentLine(base, cur)
		r.add(Finding{
			Rule: ruleAppendOnly, Code: "append_only/rewritten", Severity: SeverityRed, Path: e.rel, Line: line + n,
			Message: fmt.Sprintf("append-only violation: content committed at %s was modified", ref),
			Detail:  fmt.Sprintf("was: %s\nnow: %s", was, now),
		})
	}

	// Pass 3: files with no baseline, minus rotation targets.
	for _, e := range fresh {
		if rotatedInto[e.rel] {
			continue
		}
		r.add(Finding{
			Rule: ruleAppendOnly, Code: "append_only/no-baseline", Severity: SeverityYellow, Path: e.rel,
			Message: "no git baseline: append-only not verified",
			Detail:  e.baselineErr.Error(),
		})
	}
}

// appendOnlyFile is one listed file with both sides of its comparison.
type appendOnlyFile struct {
	rel         string
	baseline    []byte
	baselineErr error
	current     []byte
	readErr     error
}

// rotation describes a cut span that was found verbatim in another file.
type rotation struct {
	dst       string
	firstLine int // 1-based line of the cut's first line, within the header-stripped body
	lines     int
}

// findRotation locates the span of base that is missing from cur and looks
// for it, whole-line and verbatim, after the header of each candidate.
// Candidates are searched in config order; the first hit wins.
func findRotation(base, cur []byte, candidates []*appendOnlyFile, headerFor func(string) int) (rotation, bool) {
	span, first, lines, ok := cutSpan(base, cur)
	if !ok || isBlank(span) {
		return rotation{}, false
	}
	needle := lineAligned(span)
	for _, c := range candidates {
		if c.readErr != nil {
			continue
		}
		if bytes.Contains(lineAligned(stripHeader(c.current, headerFor(c.rel))), needle) {
			return rotation{dst: c.rel, firstLine: first, lines: lines}, true
		}
	}
	return rotation{}, false
}

// cutSpan splits base as keep + cut + tail, where keep is the longest run of
// leading lines shared with cur and tail is the shortest line-aligned suffix
// of base that the rest of cur still begins with (same trailing-newline
// tolerance as the prefix check). It returns cut, its 1-based first line, and
// its line count. ok is false when nothing was cut — every baseline line is
// still present and the divergence is inside the last line.
func cutSpan(base, cur []byte) (span []byte, firstLine, lines int, ok bool) {
	bo := lineOffsets(base)
	co := lineOffsets(cur)
	i := 0
	for i < len(bo)-1 && i < len(co)-1 && bytes.Equal(base[bo[i]:bo[i+1]], cur[co[i]:co[i+1]]) {
		i++
	}
	if i >= len(bo)-1 {
		return nil, 0, 0, false
	}
	var rest []byte
	if i < len(co)-1 {
		rest = cur[co[i]:]
	}
	for j := i + 1; j < len(bo); j++ {
		if hasAppendOnlyPrefix(rest, base[bo[j]:]) {
			return base[bo[i]:bo[j]], i + 1, j - i, true
		}
	}
	return nil, 0, 0, false // unreachable: j == len(bo)-1 yields an empty tail
}

// lineOffsets returns the byte offset of every line start plus len(data), so
// line k is data[off[k]:off[k+1]] and there are len(off)-1 lines.
func lineOffsets(data []byte) []int {
	off := []int{0}
	for k, b := range data {
		if b == '\n' {
			off = append(off, k+1)
		}
	}
	if off[len(off)-1] != len(data) {
		off = append(off, len(data))
	}
	return off
}

// stripHeader drops the first n lines. Fewer than n lines leaves nothing.
func stripHeader(data []byte, n int) []byte {
	if n <= 0 {
		return data
	}
	off := lineOffsets(data)
	if n >= len(off)-1 {
		return nil
	}
	return data[off[n]:]
}

// lineAligned frames data with newlines so a Contains match can only start
// and end on line boundaries.
func lineAligned(data []byte) []byte {
	out := make([]byte, 0, len(data)+2)
	out = append(out, '\n')
	out = append(out, data...)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}

// isBlank reports whether data holds no visible character: a deleted blank
// line is not a move, whatever else happens to contain a blank line.
func isBlank(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

// hasAppendOnlyPrefix reports whether current still starts with baseline.
//
// The trailing-newline tolerance is specified: a baseline whose final newline
// was dropped before new content was appended is still an append. Note what
// that necessarily admits -- dropping the final newline and continuing to write
// is byte-for-byte identical to extending the baseline's last line, so the last
// line may grow. The two cases cannot be told apart, and both are appends.
// Everything before that last line remains strictly immutable: no earlier byte
// may change, and nothing may be deleted.
func hasAppendOnlyPrefix(current, baseline []byte) bool {
	if bytes.HasPrefix(current, baseline) {
		return true
	}
	return bytes.HasPrefix(current, bytes.TrimSuffix(baseline, []byte("\n")))
}

// ValidateBaseRef reports whether ref resolves to a commit in root's
// repository. The CLI calls this at startup when --base is given: an explicit
// baseline demand that cannot be honored is a usage error, not a finding.
func ValidateBaseRef(root, ref string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("cannot resolve --base %s: git executable not found in PATH", ref)
	}
	cmd := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot resolve --base %s: %s", ref, gitErrorReason(stderr.String(), err))
	}
	return nil
}

// gitShowRef reads <file> as committed at ref. The <ref>:./<path> form
// resolves the path relative to -C, which is what makes this work when the
// memlint root is a subdirectory of a larger repository.
func gitShowRef(root, ref, rel string) ([]byte, error) {
	spec := ref + ":./" + filepath.ToSlash(rel)
	cmd := exec.Command("git", "-C", root, "show", spec)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s", gitErrorReason(stderr.String(), err))
	}
	return stdout.Bytes(), nil
}

// gitErrorReason turns git's stderr into one readable line.
func gitErrorReason(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "fatal: ")
		line = strings.TrimPrefix(line, "error: ")
		if line != "" {
			return line
		}
	}
	return err.Error()
}

// firstDivergentLine locates the first line where current stops matching
// baseline, returning a 1-based line number and both sides for the report.
func firstDivergentLine(baseline, current []byte) (int, string, string) {
	base := splitLines(baseline)
	cur := splitLines(current)

	for i, want := range base {
		if i >= len(cur) {
			return i + 1, echo(want), "<EOF>"
		}
		if cur[i] != want {
			return i + 1, echo(want), echo(cur[i])
		}
	}
	// Every baseline line matched yet the byte prefix did not: the divergence is
	// in trailing bytes of the final line. Report that line.
	n := len(base)
	if n == 0 {
		return 1, "<EOF>", echo(firstLine(cur))
	}
	nowLine := "<EOF>"
	if n-1 < len(cur) {
		nowLine = echo(cur[n-1])
	}
	return n, echo(base[n-1]), nowLine
}

// splitLines splits on \n without inventing a trailing empty line for content
// that simply ends with a newline.
func splitLines(data []byte) []string {
	s := string(data)
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return "<EOF>"
	}
	return lines[0]
}

func echo(s string) string {
	s = strings.TrimSuffix(s, "\r")
	if len(s) > maxLineEcho {
		return s[:maxLineEcho] + "..."
	}
	return s
}
