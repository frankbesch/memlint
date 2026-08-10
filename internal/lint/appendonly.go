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
func checkAppendOnly(r *runner, cfg *config.AppendOnly) {
	ref := cfg.BaseRef
	if ref == "" {
		ref = "HEAD"
	}

	gitAvailable := true
	if _, err := exec.LookPath("git"); err != nil {
		gitAvailable = false
	}

	for _, f := range cfg.Files {
		rel := config.CleanRel(f)
		if !gitAvailable {
			r.add(Finding{
				Rule: ruleAppendOnly, Code: "append_only/no-baseline", Severity: SeverityYellow, Path: rel,
				Message: "no git baseline: append-only not verified",
				Detail:  "git executable not found in PATH",
			})
			continue
		}
		checkAppendOnlyFile(r, rel, ref)
	}
}

func checkAppendOnlyFile(r *runner, rel, ref string) {
	abs, ok := r.resolve(rel)
	if !ok {
		r.red(ruleAppendOnly, "append_only/escape", rel, "path escapes the repository root")
		return
	}

	baseline, gitErr := gitShowRef(r.root, ref, rel)
	if gitErr != nil {
		// No baseline is not a violation. It means the invariant could not be
		// established, which the spec assigns YELLOW: an unversioned file has
		// nothing to have diverged from.
		r.add(Finding{
			Rule: ruleAppendOnly, Code: "append_only/no-baseline", Severity: SeverityYellow, Path: rel,
			Message: "no git baseline: append-only not verified",
			Detail:  gitErr.Error(),
		})
		return
	}

	current, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			r.red(ruleAppendOnly, "append_only/missing", rel, fmt.Sprintf(
				"file exists at %s but is missing from the working tree", ref))
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
		Rule: ruleAppendOnly, Code: "append_only/rewritten", Severity: SeverityRed, Path: rel, Line: line,
		Message: fmt.Sprintf("append-only violation: content committed at %s was modified", ref),
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
