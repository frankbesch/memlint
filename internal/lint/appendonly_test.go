package lint

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

// runAppendOnly evaluates only the append_only rule against root.
func runAppendOnly(root string, files ...string) Result {
	return Run(root, &config.Config{AppendOnly: &config.AppendOnly{Files: files}})
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
// v0.1 rule: it compares the git HEAD blob against the working tree, so it is a
// working-tree rewrite guard, NOT historical immutability enforcement. Once a
// rewrite is committed it becomes the new baseline and this rule goes quiet.
// Catching that requires comparing against a base ref, which is roadmap.
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
