package lint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

const agentBot = "openwiki[bot]"

// runHumanBrief evaluates only the human_brief rule against root.
func runHumanBrief(root string, agents []string, files ...string) Result {
	return Run(root, &config.Config{HumanBrief: &config.HumanBrief{
		Files: files, AgentAuthors: agents,
	}})
}

// gitAs runs a git command in dir under a specific author/committer identity,
// isolated from the developer's own git config like git() in helpers.
func gitAs(t *testing.T, dir, name, email string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME="+name,
		"GIT_AUTHOR_EMAIL="+email,
		"GIT_COMMITTER_NAME="+name,
		"GIT_COMMITTER_EMAIL="+email,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// commitAs stages everything and commits it under the given identity.
func commitAs(t *testing.T, dir, name, email, msg string) {
	t.Helper()
	gitAs(t, dir, name, email, "add", "-A")
	gitAs(t, dir, name, email, "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg)
}

func TestHumanBriefCleanHistoryPasses(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
	wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 0, 0)
}

func TestHumanBriefAgentCommitByNameFails(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
	writeFile(t, root, "INSTRUCTIONS.md", "scope\nagent addition\n")
	commitAs(t, root, agentBot, "bot@example.invalid", "agent edit")

	res := runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "human-brief violation")
	f := res.Findings[0]
	if !strings.Contains(f.Detail, agentBot) {
		t.Errorf("detail should name the offending author, got %q", f.Detail)
	}
	if !strings.Contains(f.Detail, "commit ") {
		t.Errorf("detail should name the offending commit, got %q", f.Detail)
	}
}

func TestHumanBriefAgentCommitByEmailFails(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
	writeFile(t, root, "INSTRUCTIONS.md", "scope\nagent addition\n")
	commitAs(t, root, "Innocuous Name", "agent@bots.invalid", "agent edit")

	res := runHumanBrief(root, []string{"agent@bots.invalid"}, "INSTRUCTIONS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "human-brief violation")
}

func TestHumanBriefMatchIsCaseInsensitiveButExact(t *testing.T) {
	requireGit(t)

	t.Run("case-insensitive match", func(t *testing.T) {
		root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
		writeFile(t, root, "INSTRUCTIONS.md", "scope\nmore\n")
		commitAs(t, root, "OpenWiki[Bot]", "bot@example.invalid", "agent edit")
		wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 1, 0)
	})

	t.Run("substring is not a match", func(t *testing.T) {
		root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
		writeFile(t, root, "INSTRUCTIONS.md", "scope\nmore\n")
		commitAs(t, root, "claude reviewer", "human@example.invalid", "human edit")
		wantCounts(t, runHumanBrief(root, []string{"claude"}, "INSTRUCTIONS.md"), 0, 0)
	})
}

func TestHumanBriefCountsEarlierAgentCommits(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
	for i, line := range []string{"first\n", "second\n"} {
		writeFile(t, root, "INSTRUCTIONS.md", "scope\n"+line)
		commitAs(t, root, agentBot, "bot@example.invalid", "agent edit "+string(rune('a'+i)))
	}

	res := runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md")
	wantCounts(t, res, 1, 0)
	if d := res.Findings[0].Detail; !strings.Contains(d, "1 earlier agent-authored") {
		t.Errorf("detail should count earlier agent commits, got %q", d)
	}
}

// An agent commit to some other file must not taint the brief.
func TestHumanBriefOnlyListedFileMatters(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{
		"INSTRUCTIONS.md": "scope\n",
		"wiki/page.md":    "generated\n",
	})
	writeFile(t, root, "wiki/page.md", "regenerated\n")
	commitAs(t, root, agentBot, "bot@example.invalid", "agent regenerates its own file")

	wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 0, 0)
}

// The differentiating property against [append_only]: the violation lives in
// history, so later human commits do not launder it. TestAppendOnly-
// CommittedRewriteIsInvisible documents the opposite scope for that rule.
func TestHumanBriefAgentCommitStaysVisibleAfterHumanCommits(t *testing.T) {
	requireGit(t)
	root := newRepo(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
	writeFile(t, root, "INSTRUCTIONS.md", "scope\nagent line\n")
	commitAs(t, root, agentBot, "bot@example.invalid", "agent edit")
	writeFile(t, root, "INSTRUCTIONS.md", "scope\nagent line\nhuman line\n")
	commitAs(t, root, "memlint test", "test@example.invalid", "human edit on top")

	res := runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "human-brief violation")
}

func TestHumanBriefYellowWhenNoHistory(t *testing.T) {
	requireGit(t)

	t.Run("untracked file", func(t *testing.T) {
		root := newRepo(t, map[string]string{"README.md": "x\n"})
		writeFile(t, root, "INSTRUCTIONS.md", "scope\n")
		res := runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md")
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "no git history")
	})

	t.Run("not a git repository", func(t *testing.T) {
		root := writeTree(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
		wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 0, 1)
	})

	t.Run("repository with no HEAD", func(t *testing.T) {
		root := writeTree(t, map[string]string{"INSTRUCTIONS.md": "scope\n"})
		git(t, root, "init", "-q", "-b", "main")
		wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 0, 1)
	})
}

// Same subdirectory contract as [append_only]: the pathspec resolves relative
// to the memlint root, not the enclosing repository's root.
func TestHumanBriefRootInsideLargerRepo(t *testing.T) {
	requireGit(t)
	repo := newRepo(t, map[string]string{
		"unrelated.md":          "top level\n",
		"agent/INSTRUCTIONS.md": "scope\n",
	})
	root := filepath.Join(repo, "agent")

	wantCounts(t, runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md"), 0, 0)

	writeFile(t, repo, "agent/INSTRUCTIONS.md", "scope\nagent line\n")
	commitAs(t, repo, agentBot, "bot@example.invalid", "agent edit")
	res := runHumanBrief(root, []string{agentBot}, "INSTRUCTIONS.md")
	wantCounts(t, res, 1, 0)
	wantMessage(t, res, "human-brief violation")
}
