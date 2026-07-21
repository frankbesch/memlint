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
// content stored at HEAD.
//
// Scope, deliberately narrow in v0.1: this compares the git HEAD blob with the
// working tree. It is a working-tree rewrite guard, not historical immutability
// enforcement -- a rewrite that has already been committed becomes the new
// baseline and passes. Comparing against the index, an arbitrary base ref, or a
// pull request base is roadmap, not v0.1.
func checkAppendOnly(r *runner, cfg *config.AppendOnly) {
	gitAvailable := true
	if _, err := exec.LookPath("git"); err != nil {
		gitAvailable = false
	}

	for _, f := range cfg.Files {
		rel := config.CleanRel(f)
		if !gitAvailable {
			r.add(Finding{
				Rule: ruleAppendOnly, Severity: SeverityYellow, Path: rel,
				Message: "no git baseline: append-only not verified",
				Detail:  "git executable not found in PATH",
			})
			continue
		}
		checkAppendOnlyFile(r, rel)
	}
}

func checkAppendOnlyFile(r *runner, rel string) {
	abs, ok := r.resolve(rel)
	if !ok {
		r.red(ruleAppendOnly, rel, "path escapes the repository root")
		return
	}

	baseline, gitErr := gitShowHead(r.root, rel)
	if gitErr != nil {
		// No baseline is not a violation. It means the invariant could not be
		// established, which the spec assigns YELLOW: an unversioned file has
		// nothing to have diverged from.
		r.add(Finding{
			Rule: ruleAppendOnly, Severity: SeverityYellow, Path: rel,
			Message: "no git baseline: append-only not verified",
			Detail:  gitErr.Error(),
		})
		return
	}

	current, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			r.red(ruleAppendOnly, rel, "file exists at HEAD but is missing from the working tree")
			return
		}
		r.cannotVerify(ruleAppendOnly, rel, err)
		return
	}
	r.mark(rel)

	if hasAppendOnlyPrefix(current, baseline) {
		return
	}

	line, was, now := firstDivergentLine(baseline, current)
	r.add(Finding{
		Rule: ruleAppendOnly, Severity: SeverityRed, Path: rel, Line: line,
		Message: "append-only violation: content committed at HEAD was modified",
		Detail:  fmt.Sprintf("was: %s\nnow: %s", was, now),
	})
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

// gitShowHead reads <file> as committed at HEAD. The HEAD:./<path> form
// resolves the path relative to -C, which is what makes this work when the
// memlint root is a subdirectory of a larger repository.
func gitShowHead(root, rel string) ([]byte, error) {
	spec := "HEAD:./" + filepath.ToSlash(rel)
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
