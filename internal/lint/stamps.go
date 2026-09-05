package lint

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleStamps = "stamps"

// checkStamps verifies that each listed file carries a "last verified" date
// stamp no older than max_age_days relative to the file's last change — the
// last commit touching it, or now if it has uncommitted edits. A stamp that
// predates the content it vouches for is evidence that has quietly expired,
// which is what a hand-kept stale-artifact register exists to catch.
func checkStamps(r *runner, cfg *config.Stamps) {
	re := regexp.MustCompile(cfg.EffectivePattern())
	gitOK := true
	if _, err := exec.LookPath("git"); err != nil {
		gitOK = false
	}
	for _, rel := range expandSources(r, ruleStamps, cfg.Files, Finding{
		Rule: ruleStamps, Code: "stamps/no-match", Severity: SeverityYellow,
		Message: "files glob matched no files",
		Detail:  "a stale files glob is stamp coverage that silently never runs",
	}) {
		if r.unchanged(rel) {
			continue
		}
		abs, ok := r.resolve(rel)
		if !ok {
			r.red(ruleStamps, "stamps/escape", rel, "path escapes the repository root")
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				r.red(ruleStamps, "stamps/missing-source", rel, "stamped file does not exist")
			} else {
				r.cannotVerify(ruleStamps, rel, err)
			}
			continue
		}
		r.mark(rel)

		stamp, line, found := findStamp(re, string(data))
		if !found {
			r.add(Finding{
				Rule: ruleStamps, Code: "stamps/missing", Severity: SeverityRed, Path: rel,
				Message: "no stamp found: file is declared to carry a last-verified date",
				Detail:  fmt.Sprintf("pattern: %s", cfg.EffectivePattern()),
			})
			continue
		}
		stampDate, err := time.Parse("2006-01-02", stamp)
		if err != nil {
			r.add(Finding{
				Rule: ruleStamps, Code: "stamps/unparsable", Severity: SeverityRed, Path: rel, Line: line,
				Message: fmt.Sprintf("stamp %q is not a YYYY-MM-DD date", stamp),
			})
			continue
		}

		changed, why, ok := lastChange(r.root, rel, gitOK)
		if !ok {
			r.add(Finding{
				Rule: ruleStamps, Code: "stamps/no-baseline", Severity: SeverityYellow, Path: rel, Line: line,
				Message: "no git history: stamp age not verified",
				Detail:  why,
			})
			continue
		}
		age := changed.Sub(stampDate)
		if age > time.Duration(cfg.MaxAgeDays)*24*time.Hour {
			r.add(Finding{
				Rule: ruleStamps, Code: "stamps/stale", Severity: SeverityYellow, Path: rel, Line: line,
				Message: fmt.Sprintf("stamp %s is %d days older than the file's last change (max %d)",
					stamp, int(age.Hours()/24), cfg.MaxAgeDays),
				Detail: "last change: " + changed.UTC().Format("2006-01-02") + " (" + why + ")",
			})
		}
	}
}

// findStamp returns the first capture group of the first matching line.
func findStamp(re *regexp.Regexp, content string) (string, int, bool) {
	for i, line := range strings.Split(content, "\n") {
		if m := re.FindStringSubmatch(line); m != nil && len(m) > 1 {
			return m[1], i + 1, true
		}
	}
	return "", 0, false
}

// lastChange is when rel last changed: now if the working copy differs from
// HEAD (or is untracked with history absent), else the last commit's author
// date. why describes which.
func lastChange(root, rel string, gitOK bool) (time.Time, string, bool) {
	if !gitOK {
		return time.Time{}, "git executable not found in PATH", false
	}
	out, err := gitOut(root, "log", "-1", "--format=%at", "--", "./"+filepath.ToSlash(rel))
	if err != nil {
		return time.Time{}, err.Error(), false
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return time.Time{}, "no commits touch this file", false
	}
	secs, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return time.Time{}, "unexpected git log output: " + out, false
	}
	// Uncommitted edits: the file changed after its last commit, i.e. now.
	if _, err := gitOut(root, "diff", "--quiet", "HEAD", "--", "./"+filepath.ToSlash(rel)); err != nil {
		return time.Now(), "uncommitted edits", true
	}
	return time.Unix(secs, 0), "last commit", true
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", gitErrorReason(stderr.String(), err))
	}
	return stdout.String(), nil
}
