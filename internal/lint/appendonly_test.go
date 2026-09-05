package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

// runAppendOnly evaluates only the append_only rule against root.
func runAppendOnly(root string, files ...string) Result {
	return Run(root, &config.Config{AppendOnly: &config.AppendOnly{Files: files}})
}

// runAppendOnlyBase is runAppendOnly with an explicit --base ref.
func runAppendOnlyBase(root, base string, files ...string) Result {
	return Run(root, &config.Config{AppendOnly: &config.AppendOnly{Files: files, BaseRef: base}})
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// git runs a git command in dir, isolated from the developer's own git config
// so that global hooks, commit signing, or templates cannot change the result.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=memlint test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=memlint test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a git repository containing files, with everything committed.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := writeTree(t, files)
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "add", "-A")
	git(t, root, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "baseline")
	return root
}

const baseline = "# Decisions\n\n- D-001: adopt memlint\n"

func TestAppendOnlyAppendPasses(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	writeFile(t, root, "memory/decisions.md", baseline+"- D-002: ship v0.1\n")

	wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 0)
}

func TestAppendOnlyRewriteFails(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	writeFile(t, root, "memory/decisions.md", "# Decisions\n\n- D-001: adopt something else\n")

	res := runAppendOnly(root, "memory/decisions.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "append-only violation")
	f := res.Findings[0]
	if f.Line != 3 {
		t.Errorf("got line %d, want 3 (the first divergent line)", f.Line)
	}
	if want := "was: - D-001: adopt memlint\nnow: - D-001: adopt something else"; f.Detail != want {
		t.Errorf("detail:\n%s\nwant:\n%s", f.Detail, want)
	}
}

func TestAppendOnlyTruncationFails(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	writeFile(t, root, "memory/decisions.md", "# Decisions\n")

	res := runAppendOnly(root, "memory/decisions.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "append-only violation")
	if got := res.Findings[0].Detail; got != "was: \nnow: <EOF>" {
		t.Errorf("truncation should report <EOF>, got %q", got)
	}
}

func TestAppendOnlyDeletedFileFails(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	if err := os.Remove(filepath.Join(root, "memory", "decisions.md")); err != nil {
		t.Fatal(err)
	}

	res := runAppendOnly(root, "memory/decisions.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "missing from the working tree")
}

// The spec's tolerance: a baseline whose trailing newline was dropped before
// new content was appended is still an append, not a rewrite.
//
// Extending the last line is the same tolerance seen from the other side --
// "drop the newline, then append" and "extend the last line" produce identical
// bytes, so admitting the first necessarily admits the second. What the
// tolerance does not admit is any change to content before that last line.
func TestAppendOnlyTrailingNewlineTolerance(t *testing.T) {
	requireGit(t)

	cases := []struct {
		name    string
		base    string
		working string
		wantRed int
	}{
		{"newline dropped, nothing added", "- D-001\n", "- D-001", 0},
		{"newline dropped, more appended", "- D-001\n", "- D-001 continued\n", 0},
		{"plain append", "- D-001\n", "- D-001\n- D-002\n", 0},
		{"last line's existing text changed", "- D-001\n", "- D-009\n", 1},
		{"earlier line changed", "- D-001\n- D-002\n", "- D-009\n- D-002\n", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t, map[string]string{"memory/decisions.md": tc.base})
			writeFile(t, root, "memory/decisions.md", tc.working)
			wantCounts(t, runAppendOnly(root, "memory/decisions.md"), tc.wantRed, 0)
		})
	}
}

func TestAppendOnlyYellowWhenNoBaseline(t *testing.T) {
	requireGit(t)

	t.Run("untracked file", func(t *testing.T) {
		root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
		writeFile(t, root, "memory/untracked.md", "new\n")
		res := runAppendOnly(root, "memory/untracked.md")
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "no git baseline")
	})

	t.Run("not a git repository", func(t *testing.T) {
		root := writeTree(t, map[string]string{"memory/decisions.md": baseline})
		wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 1)
	})

	t.Run("repository with no HEAD", func(t *testing.T) {
		root := writeTree(t, map[string]string{"memory/decisions.md": baseline})
		git(t, root, "init", "-q", "-b", "main")
		wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 1)
	})
}

// The memlint root is frequently a subdirectory of a larger repository. The
// HEAD:./<path> form is what makes the baseline lookup resolve relative to the
// target root rather than the repository root.
func TestAppendOnlyRootInsideLargerRepo(t *testing.T) {
	requireGit(t)
	repo := newRepo(t, map[string]string{
		"unrelated.md":              "top level\n",
		"agent/memory/decisions.md": baseline,
	})
	root := filepath.Join(repo, "agent")

	wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 0)

	writeFile(t, repo, "agent/memory/decisions.md", "rewritten\n")
	res := runAppendOnly(root, "memory/decisions.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "append-only violation")
}

// TestAppendOnlyCommittedRewriteIsInvisible documents the exact scope of the
// DEFAULT rule: it compares the git HEAD blob against the working tree, so it
// is a working-tree rewrite guard, NOT historical immutability enforcement.
// Once a rewrite is committed it becomes the new baseline and this rule goes
// quiet. Catching that is what --base is for; see the tests below.
func TestAppendOnlyCommittedRewriteIsInvisible(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})

	writeFile(t, root, "memory/decisions.md", "# Decisions\n\n- D-001: rewritten\n")
	if res := runAppendOnly(root, "memory/decisions.md"); res.Red() != 1 {
		t.Fatalf("uncommitted rewrite should be caught, got:\n%s", dump(res))
	}

	git(t, root, "add", "-A")
	git(t, root, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "rewrite history")

	wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 0)
}

// commit stages and commits everything in dir, mirroring how newRepo commits.
func commit(t *testing.T, dir, msg string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg)
}

// The point of --base: a rewrite that is already COMMITTED — invisible to the
// default HEAD comparison, and to any CI checkout — diverges from the base
// branch's blob and is caught. This is the pull-request-gate scenario.
func TestAppendOnlyBaseRefCatchesCommittedRewrite(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	git(t, root, "branch", "base")

	writeFile(t, root, "memory/decisions.md", "# Decisions\n\n- D-001: rewritten\n")
	commit(t, root, "rewrite, committed like a PR would")

	// Default scope stays quiet — that limitation is pinned above.
	wantCounts(t, runAppendOnly(root, "memory/decisions.md"), 0, 0)

	res := runAppendOnlyBase(root, "base", "memory/decisions.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "append-only violation: content committed at base was modified")
}

func TestAppendOnlyBaseRefCommittedAppendPasses(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	git(t, root, "branch", "base")

	writeFile(t, root, "memory/decisions.md", baseline+"- D-002: appended in the PR\n")
	commit(t, root, "append, committed")

	wantCounts(t, runAppendOnlyBase(root, "base", "memory/decisions.md"), 0, 0)
}

// A file created after the base ref has no baseline there — nothing to have
// diverged from — so it is YELLOW, exactly like an untracked file under HEAD.
func TestAppendOnlyBaseRefNewFileIsYellow(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})
	git(t, root, "branch", "base")

	writeFile(t, root, "memory/new-log.md", "born after base\n")
	commit(t, root, "new file")

	res := runAppendOnlyBase(root, "base", "memory/new-log.md")
	wantCounts(t, res, 0, 1)
	wantMessage(t, res, "no git baseline")
}

func TestValidateBaseRef(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": baseline})

	if err := ValidateBaseRef(root, "main"); err != nil {
		t.Errorf("existing branch should validate, got: %v", err)
	}
	if err := ValidateBaseRef(root, "no-such-ref"); err == nil {
		t.Error("unknown ref must be rejected")
	}
	if err := ValidateBaseRef(t.TempDir(), "main"); err == nil {
		t.Error("a directory that is not a git repository must be rejected")
	}
}

// runAppendOnlyCfg evaluates append_only with a full section, for the
// header_lines cases below.
func runAppendOnlyCfg(root string, cfg *config.AppendOnly) Result {
	return Run(root, &config.Config{AppendOnly: cfg})
}

// infoFindings returns the INFO findings of a result, in order.
func infoFindings(res Result) []Finding {
	var out []Finding
	for _, f := range res.Findings {
		if f.Severity == SeverityInfo {
			out = append(out, f)
		}
	}
	return out
}

const logHeader = "# Decisions\nRead memory/index.md first.\n\n"
const logBody = "---\n- D-001: adopt memlint\n- D-002: ship v0.1\n- D-003: rotate at three\n- D-004: keep going\n"

// header_lines marks the first N lines as the ONLY mutable span. A pointer
// header that changes is not a rewrite; a body line that changes still is.
func TestAppendOnlyHeaderLinesExemptHeaderOnly(t *testing.T) {
	requireGit(t)
	base := logHeader + logBody

	cases := []struct {
		name        string
		headerLines int
		working     string
		wantRed     int
	}{
		{"header changed, header_lines covers it", 3, "# Decisions\nRead memory/decisions-index.md first.\n\n" + logBody, 0},
		{"header changed, header_lines absent", 0, "# Decisions\nRead memory/decisions-index.md first.\n\n" + logBody, 1},
		{"header line added within the window", 3, "# Decisions\nVolumes: none yet\n\n" + logBody, 0},
		{"body changed past the header", 3, logHeader + "---\n- D-001: adopt something else\n- D-002: ship v0.1\n- D-003: rotate at three\n- D-004: keep going\n", 1},
		{"body truncated past the header", 3, logHeader + "---\n- D-001: adopt memlint\n", 1},
		{"append after the header", 3, base + "- D-005: appended\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t, map[string]string{"memory/decisions.md": base})
			writeFile(t, root, "memory/decisions.md", tc.working)
			res := runAppendOnlyCfg(root, &config.AppendOnly{Files: []string{"memory/decisions.md"}, HeaderLines: tc.headerLines})
			wantCounts(t, res, tc.wantRed, 0)
			if len(infoFindings(res)) != 0 {
				t.Errorf("no rotation happened, so no INFO expected:\n%s", dump(res))
			}
		})
	}
}

// A body change under header_lines reports its line number in the full file,
// not relative to the stripped body.
func TestAppendOnlyHeaderLinesReportsFullFileLine(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", logHeader+"---\n- D-001: adopt memlint\n- D-002: ship v0.2\n- D-003: rotate at three\n- D-004: keep going\n")
	res := runAppendOnlyCfg(root, &config.AppendOnly{Files: []string{"memory/decisions.md"}, HeaderLines: 3})
	wantCounts(t, res, 1, 0)
	if got := res.Findings[0].Line; got != 6 {
		t.Errorf("got line %d, want 6 (line 3 of the body, after a 3-line header)", got)
	}
}

// The rotation shape: baseline = header + D-001..D-004; working copy = new
// header + D-003..D-004 (+ an append); the cut span D-001..D-002 reappears
// verbatim, after the header, in a declared append_only file that has no
// baseline of its own. That is INFO append_only/rotated, and nothing else.
const rotatedLive = "# Decisions\nVolume 1 lives in memory/archive/vol1.md.\n\n---\n- D-003: rotate at three\n- D-004: keep going\n- D-005: appended after rotation\n"
const rotatedArchive = "# Decisions, volume 1\nArchived verbatim.\n\n---\n- D-001: adopt memlint\n- D-002: ship v0.1\n"

func rotationCfg() *config.AppendOnly {
	return &config.AppendOnly{Files: []string{"memory/decisions.md", "memory/archive/vol1.md"}, HeaderLines: 3}
}

func wantRotated(t *testing.T, res Result, src, dst string, lines int) {
	t.Helper()
	wantCounts(t, res, 0, 0)
	infos := infoFindings(res)
	if len(infos) != 1 {
		t.Fatalf("want exactly one INFO finding, got %d:\n%s", len(infos), dump(res))
	}
	f := infos[0]
	if f.Code != "append_only/rotated" || f.Rule != ruleAppendOnly {
		t.Errorf("got %s %s, want append_only append_only/rotated", f.Rule, f.Code)
	}
	if f.Path != src || f.RelatedPath != dst {
		t.Errorf("got %s -> %s, want %s -> %s", f.Path, f.RelatedPath, src, dst)
	}
	want := fmt.Sprintf("%d lines moved verbatim", lines)
	if !strings.Contains(f.Message, dst) || !strings.Contains(f.Message, want) {
		t.Errorf("message %q must name %s and %q", f.Message, dst, want)
	}
}

func TestAppendOnlyRotationIsInfo(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", rotatedLive)
	writeFile(t, root, "memory/archive/vol1.md", rotatedArchive)

	res := runAppendOnlyCfg(root, rotationCfg())
	wantRotated(t, res, "memory/decisions.md", "memory/archive/vol1.md", 2)
	if res.FilesChecked != 2 {
		t.Errorf("both files were inspected, got FilesChecked=%d", res.FilesChecked)
	}
}

// Without header_lines the rotation still passes when the header did not
// change: the moved span is exactly the cut lines.
func TestAppendOnlyRotationWithoutHeaderLines(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", logHeader+"---\n- D-003: rotate at three\n- D-004: keep going\n")
	writeFile(t, root, "memory/archive/vol1.md", rotatedArchive)

	res := runAppendOnlyCfg(root, &config.AppendOnly{Files: []string{"memory/decisions.md", "memory/archive/vol1.md"}})
	wantRotated(t, res, "memory/decisions.md", "memory/archive/vol1.md", 2)
}

// The allowance withdraws — the existing RED, unchanged — whenever the cut
// span is not found verbatim in an eligible destination.
func TestAppendOnlyRotationWithdraws(t *testing.T) {
	requireGit(t)

	cases := []struct {
		name    string
		archive string // "" = no archive written
		files   []string
	}{
		{"moved line altered", "# Decisions, volume 1\nArchived verbatim.\n\n---\n- D-001: adopt memlint\n- D-002: ship v0.2\n", nil},
		{"moved line missing", "# Decisions, volume 1\nArchived verbatim.\n\n---\n- D-001: adopt memlint\n", nil},
		{"span only inside the destination header", "---\n- D-001: adopt memlint\n- D-002: ship v0.1\n", nil},
		{"destination not declared append_only", rotatedArchive, []string{"memory/decisions.md"}},
		{"no destination at all", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
			writeFile(t, root, "memory/decisions.md", rotatedLive)
			if tc.archive != "" {
				writeFile(t, root, "memory/archive/vol1.md", tc.archive)
			}
			cfg := rotationCfg()
			if tc.files != nil {
				cfg.Files = tc.files
			}
			res := runAppendOnlyCfg(root, cfg)
			if res.Red() != 1 {
				t.Errorf("want the plain RED rewrite finding, got:\n%s", dump(res))
			}
			if !hasFinding(res, ruleAppendOnly, "memory/decisions.md", "append-only violation") {
				t.Errorf("missing append_only/rewritten on the live log:\n%s", dump(res))
			}
			if len(infoFindings(res)) != 0 {
				t.Errorf("no INFO when the allowance does not apply:\n%s", dump(res))
			}
		})
	}
}

// A destination that already has a baseline is not a rotation target: the
// span would have to have been appended to a committed volume, which is a
// separate append_only check of its own, and a verbatim copy there does not
// excuse deleting it from the source.
func TestAppendOnlyRotationDestinationMustBeNewAtBaseline(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{
		"memory/decisions.md":    logHeader + logBody,
		"memory/archive/vol1.md": rotatedArchive,
	})
	writeFile(t, root, "memory/decisions.md", rotatedLive)

	res := runAppendOnlyCfg(root, rotationCfg())
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "append-only violation")
	if len(infoFindings(res)) != 0 {
		t.Errorf("committed destination must not count as a rotation target:\n%s", dump(res))
	}
}

// The uncommitted destination's own "no baseline" YELLOW is withdrawn by a
// matched rotation: the moved span IS its baseline. An untracked declared
// file that is NOT the target of a rotation stays YELLOW as before.
func TestAppendOnlyRotationTargetHasNoYellow(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", rotatedLive)
	writeFile(t, root, "memory/archive/vol1.md", rotatedArchive)
	writeFile(t, root, "memory/archive/vol2.md", "# unrelated, untracked\n")

	cfg := rotationCfg()
	cfg.Files = append(cfg.Files, "memory/archive/vol2.md")
	res := runAppendOnlyCfg(root, cfg)
	wantCounts(t, res, 0, 1)
	if !hasFinding(res, ruleAppendOnly, "memory/archive/vol2.md", "no git baseline") {
		t.Errorf("the untracked non-target must keep its YELLOW:\n%s", dump(res))
	}
	if len(infoFindings(res)) != 1 {
		t.Errorf("want one rotation INFO:\n%s", dump(res))
	}
}

// Under --base the destination is "new at baseline" when the base ref has no
// blob for it, even though HEAD does: a rotation committed inside a PR passes
// the PR gate the same way an uncommitted one passes the local guard.
func TestAppendOnlyRotationUnderBaseRef(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	git(t, root, "branch", "base")
	writeFile(t, root, "memory/decisions.md", rotatedLive)
	writeFile(t, root, "memory/archive/vol1.md", rotatedArchive)
	commit(t, root, "rotate the log")

	// Default scope: HEAD equals the working tree, so nothing to say —
	// the archive has a HEAD blob and the live log matches its own.
	wantCounts(t, runAppendOnlyCfg(root, rotationCfg()), 0, 0)

	cfg := rotationCfg()
	cfg.BaseRef = "base"
	wantRotated(t, runAppendOnlyCfg(root, cfg), "memory/decisions.md", "memory/archive/vol1.md", 2)
}

// The moved span may sit anywhere after the destination's header, so a
// volume that opens with its own separator or note still qualifies, and the
// span is matched on whole lines: a partial line is not a verbatim move.
func TestAppendOnlyRotationSpanIsWholeLines(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", rotatedLive)
	writeFile(t, root, "memory/archive/vol1.md", "# vol 1\nnote\n\nlead-in\n---\n- D-001: adopt memlint\n- D-002: ship v0.1\ntrailer\n")

	res := runAppendOnlyCfg(root, rotationCfg())
	wantRotated(t, res, "memory/decisions.md", "memory/archive/vol1.md", 2)

	writeFile(t, root, "memory/archive/vol1.md", "# vol 1\nnote\n\n---\nX- D-001: adopt memlint\n- D-002: ship v0.1\n")
	res = runAppendOnlyCfg(root, rotationCfg())
	wantCounts(t, res, 1, 1)
}

// headers = { path = N } overrides header_lines per file, so an archive
// volume with a 6-line header is not left with body lines in the shared
// window. Keys must name listed files (config error otherwise).
func TestAppendOnlyPerFileHeaders(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"memory/decisions.md": logHeader + logBody})
	writeFile(t, root, "memory/decisions.md", rotatedLive)
	// A 5-line archive header: the shared N of 3 would leave "lead" and "---"
	// inside the body and the span would still be found after them; the point
	// of the override is that the body starts where THIS file's header ends.
	writeFile(t, root, "memory/archive/vol1.md", "# vol 1\nnote\n\nlead\n---\n- D-001: adopt memlint\n- D-002: ship v0.1\n")

	cfg := rotationCfg()
	cfg.Headers = map[string]int{"memory/archive/vol1.md": 5}
	wantRotated(t, runAppendOnlyCfg(root, cfg), "memory/decisions.md", "memory/archive/vol1.md", 2)

	// The override applies to the file's own prefix check as well.
	root = newRepo(t, map[string]string{"memory/archive/vol1.md": "h1\nh2\nh3\nh4\nh5\n- D-001\n"})
	writeFile(t, root, "memory/archive/vol1.md", "H1\nH2\nH3\nH4\nH5\n- D-001\n- D-002\n")
	cfg = &config.AppendOnly{Files: []string{"memory/archive/vol1.md"}, HeaderLines: 1, Headers: map[string]int{"memory/archive/vol1.md": 5}}
	wantCounts(t, runAppendOnlyCfg(root, cfg), 0, 0)
	cfg.Headers = nil
	wantCounts(t, runAppendOnlyCfg(root, cfg), 1, 0)
}
