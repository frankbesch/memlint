package lint

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleHumanBrief = "human_brief"

// checkHumanBrief verifies that no commit in a file's git history was authored
// -- or co-authored -- by a configured agent identity. The human brief is the
// one file an agent reads for scope and priorities but must never write; a
// single agent-authored commit means intent and output are no longer on
// opposite sides of that line.
//
// Both the commit author and every Co-Authored-By trailer are checked. The
// trailer is the common case, not an edge one: assisted commits routinely land
// with a human author and a `Co-Authored-By: <agent>` line, so matching the
// author alone would pass exactly the commits this rule exists to catch.
//
// Unlike [append_only], which deliberately compares only HEAD against the
// working tree, this rule scans the full history. That is not scope creep:
// authorship is simply not readable from the working tree -- a file's bytes do
// not say who wrote them -- so history is the only place this invariant lives.
// It also means a violation stays visible after later human commits, which is
// the point: the breach of trust does not age out.
//
// Matching is against name and email, case-insensitively and exactly. Renames
// are not followed; history before a rename belongs to the old path.
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

	commits, err := gitFileCommits(r.root, rel)
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

	var offending []breach
	for _, c := range commits {
		if p, id, ok := matchCommit(c, agents); ok {
			offending = append(offending, breach{hash: c.hash, who: p, id: id})
		}
	}
	if len(offending) == 0 {
		return
	}

	// git log is newest-first, so offending[0] is the most recent breach.
	latest := offending[0]
	verb := "by"
	if latest.who.role == roleCoAuthor {
		verb = "co-authored by"
	}
	detail := fmt.Sprintf("commit %s %s %s <%s> matches agent identity %q",
		shortHash(latest.hash), verb, latest.who.name, latest.who.email, latest.id)
	if n := len(offending) - 1; n > 0 {
		detail += fmt.Sprintf("\nand %d earlier agent-authored commit(s)", n)
	}
	r.add(Finding{
		Rule: ruleHumanBrief, Severity: SeverityRed, Path: rel,
		Message: "human-brief violation: agent-authored commit in file history",
		Detail:  detail,
	})
}

const (
	roleAuthor    = "author"
	roleCoAuthor  = "co-author"
	coAuthorTrail = "Co-authored-by"
)

// person is one identity attached to a commit, tagged with how they were
// attached so the finding can say "by" versus "co-authored by".
type person struct {
	role  string
	name  string
	email string
}

type commitInfo struct {
	hash   string
	people []person // author first, then co-authors in trailer order
}

// breach records which commit, which person, and which configured identity
// produced a match.
type breach struct {
	hash string
	who  person
	id   string
}

// matchCommit reports the first person on the commit -- author before
// co-authors -- whose name or email matches a configured agent identity.
func matchCommit(c commitInfo, agents []string) (person, string, bool) {
	for _, p := range c.people {
		for _, a := range agents {
			if strings.EqualFold(p.name, a) || (p.email != "" && strings.EqualFold(p.email, a)) {
				return p, a, true
			}
		}
	}
	return person{}, "", false
}

// commitRecordSep and commitFieldSep are ASCII record/unit separators, chosen
// because a commit message cannot contain them, so neither a multi-line body
// nor an exotic author name can desync the parse.
const (
	commitRecordSep = "\x1e"
	commitFieldSep  = "\x1f"
)

// gitFileCommits lists every commit touching rel, newest first, with each
// commit's author and any Co-Authored-By trailers. The pathspec is passed
// relative to -C, which is what makes this work when the memlint root is a
// subdirectory of a larger repository.
func gitFileCommits(root, rel string) ([]commitInfo, error) {
	// %B (raw body) carries the trailers; the record/field separators keep the
	// multi-line body from being mistaken for structure.
	format := "--format=%H" + commitFieldSep + "%an" + commitFieldSep + "%ae" +
		commitFieldSep + "%B" + commitRecordSep
	cmd := exec.Command("git", "-C", root, "log", format, "--", "./"+filepath.ToSlash(rel))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s", gitErrorReason(stderr.String(), err))
	}

	var commits []commitInfo
	for _, rec := range strings.Split(stdout.String(), commitRecordSep) {
		// git writes a newline between entries; it lands adjacent to the
		// separator, not inside a field.
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, commitFieldSep, 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected git log output: %q", rec)
		}
		c := commitInfo{
			hash:   parts[0],
			people: []person{{role: roleAuthor, name: parts[1], email: parts[2]}},
		}
		for _, co := range parseCoAuthors(parts[3]) {
			c.people = append(c.people, person{role: roleCoAuthor, name: co.name, email: co.email})
		}
		commits = append(commits, c)
	}
	return commits, nil
}

var (
	coAuthorLineRe = regexp.MustCompile(`(?im)^\s*` + regexp.QuoteMeta(coAuthorTrail) + `:\s*(.+?)\s*$`)
	nameEmailRe    = regexp.MustCompile(`^(.*?)\s*<([^>]*)>\s*$`)
)

type identity struct{ name, email string }

// parseCoAuthors extracts every Co-Authored-By trailer from a commit body as an
// (name, email) pair. A trailer without angle brackets contributes a name only.
func parseCoAuthors(body string) []identity {
	var out []identity
	for _, m := range coAuthorLineRe.FindAllStringSubmatch(body, -1) {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		if ne := nameEmailRe.FindStringSubmatch(raw); ne != nil {
			out = append(out, identity{name: strings.TrimSpace(ne[1]), email: strings.TrimSpace(ne[2])})
		} else {
			out = append(out, identity{name: raw})
		}
	}
	return out
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}
