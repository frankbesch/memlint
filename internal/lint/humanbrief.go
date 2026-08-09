package lint

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleHumanBrief = "human_brief"

// checkHumanBrief verifies that no commit in a file's git history was authored
// by a configured agent identity. The human brief is the one file an agent
// reads for scope and priorities but must never write; a single agent-authored
// commit means intent and output are no longer on opposite sides of that line.
//
// Unlike [append_only], which deliberately compares only HEAD against the
// working tree, this rule scans the full history. That is not scope creep:
// authorship is simply not readable from the working tree -- a file's bytes do
// not say who wrote them -- so history is the only place this invariant lives.
// It also means a violation stays visible after later human commits, which is
// the point: the breach of trust does not age out.
//
// Matching is against author name and author email, case-insensitively and
// exactly. Renames are not followed; history before a rename belongs to the
// old path.
func checkHumanBrief(r *runner, cfg *config.HumanBrief) {
	if _, err := exec.LookPath("git"); err != nil {
		for _, f := range cfg.Files {
			r.add(Finding{
				Rule: ruleHumanBrief, Severity: SeverityYellow, Path: config.CleanRel(f),
				Message: "no git history: human-brief authorship not verified",
				Detail:  "git executable not found in PATH",
			})
		}
		return
	}
	for _, f := range cfg.Files {
		checkHumanBriefFile(r, config.CleanRel(f), cfg.AgentAuthors)
	}
}

func checkHumanBriefFile(r *runner, rel string, agents []string) {
	if _, ok := r.resolve(rel); !ok {
		r.red(ruleHumanBrief, rel, "path escapes the repository root")
		return
	}

	commits, err := gitFileAuthors(r.root, rel)
	if err != nil {
		// Same policy as [append_only]: no history means the invariant could
		// not be established, not that it was violated.
		r.add(Finding{
			Rule: ruleHumanBrief, Severity: SeverityYellow, Path: rel,
			Message: "no git history: human-brief authorship not verified",
			Detail:  err.Error(),
		})
		return
	}
	if len(commits) == 0 {
		r.add(Finding{
			Rule: ruleHumanBrief, Severity: SeverityYellow, Path: rel,
			Message: "no git history: human-brief authorship not verified",
			Detail:  "no commits touch this file",
		})
		return
	}
	r.mark(rel)

	var offending []commitAuthor
	matched := ""
	for _, c := range commits {
		if id, ok := matchAgent(c, agents); ok {
			if len(offending) == 0 {
				matched = id
			}
			offending = append(offending, c)
		}
	}
	if len(offending) == 0 {
		return
	}

	// git log is newest-first, so offending[0] is the most recent breach.
	latest := offending[0]
	detail := fmt.Sprintf("commit %s by %s <%s> matches agent identity %q",
		shortHash(latest.hash), latest.name, latest.email, matched)
	if n := len(offending) - 1; n > 0 {
		detail += fmt.Sprintf("\nand %d earlier agent-authored commit(s)", n)
	}
	r.add(Finding{
		Rule: ruleHumanBrief, Severity: SeverityRed, Path: rel,
		Message: "human-brief violation: agent-authored commit in file history",
		Detail:  detail,
	})
}

type commitAuthor struct {
	hash, name, email string
}

// matchAgent reports whether the commit's author name or email equals one of
// the configured agent identities, and which one. Equality is case-insensitive
// but exact: "claude" must not match "claude reviewer".
func matchAgent(c commitAuthor, agents []string) (string, bool) {
	for _, a := range agents {
		if strings.EqualFold(c.name, a) || strings.EqualFold(c.email, a) {
			return a, true
		}
	}
	return "", false
}

// gitFileAuthors lists every commit touching rel, newest first. The pathspec
// is passed relative to -C, which is what makes this work when the memlint
// root is a subdirectory of a larger repository.
func gitFileAuthors(root, rel string) ([]commitAuthor, error) {
	cmd := exec.Command("git", "-C", root, "log",
		"--format=%H%x1f%an%x1f%ae", "--", "./"+filepath.ToSlash(rel))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s", gitErrorReason(stderr.String(), err))
	}

	var commits []commitAuthor
	for _, line := range strings.Split(stdout.String(), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected git log output: %q", line)
		}
		commits = append(commits, commitAuthor{hash: parts[0], name: parts[1], email: parts[2]})
	}
	return commits, nil
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}
