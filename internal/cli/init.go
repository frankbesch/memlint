package cli

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/frankbesch/memvet/internal/config"
)

// runInit inspects a repository and writes a commented starter .memvet.toml.
//
// This is the only write memvet performs, anywhere: one new file, created
// with O_EXCL so an existing config can never be overwritten, not even by a
// race. check remains strictly read-only.
//
// Rules are enabled on evidence only. An index file that exists gets
// [pointers]; observed .DS_Store or *.tmp files get [junk]. Everything else
// is emitted as a commented example, because enabling an invariant nobody
// depends on is noise, and noise is how a checker gets ignored.
func runInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print the config instead of writing it")
	root, code, done := parseCommand(fs, args, initUsage, stdout, stderr)
	if done {
		return code
	}
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(stderr, "memvet: cannot read target %s: %v\n", root, err)
		return ExitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "memvet: target %s is not a directory\n", root)
		return ExitUsage
	}

	ev := inspect(root)
	content, rep := renderConfig(ev)
	cfgPath := filepath.Join(root, config.FileName)

	if *dryRun {
		// The config goes to stdout so a script can capture it whole; the
		// report goes to stderr so it never lands in the captured file.
		fmt.Fprint(stdout, content)
		rep.write(stderr, "Would write "+cfgPath+" (dry run: nothing written)")
		return ExitClean
	}

	// O_EXCL makes "refuse to overwrite" atomic rather than check-then-write.
	f, err := os.OpenFile(cfgPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			fmt.Fprintf(stderr, "memvet: %s already exists; init refuses to overwrite it (use --dry-run to see what it would write)\n", cfgPath)
		} else {
			fmt.Fprintf(stderr, "memvet: cannot write %s: %v\n", cfgPath, err)
		}
		return ExitUsage
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		fmt.Fprintf(stderr, "memvet: writing %s: %v\n", cfgPath, err)
		return ExitUsage
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(stderr, "memvet: writing %s: %v\n", cfgPath, err)
		return ExitUsage
	}
	rep.write(stdout, "Created "+cfgPath)
	return ExitClean
}

// tierLine is one row of the init report: a rule and the evidence (or the
// reason there is none) behind its placement.
type tierLine struct{ rule, why string }

// initReport says what init inferred, what it only suspected, and what it
// refused to guess. Cautious automation earns trust by explaining itself.
type initReport struct {
	enabled     []tierLine
	suggested   []tierLine
	notInferred []tierLine
}

func (r initReport) write(w io.Writer, headline string) {
	fmt.Fprintln(w, headline)
	fmt.Fprintln(w)
	if len(r.enabled) == 0 {
		fmt.Fprintln(w, "Nothing enabled: no evidence found. check will say clean (no rules enabled).")
	} else {
		fmt.Fprintln(w, "Enabled")
		writeTier(w, r.enabled)
	}
	if len(r.suggested) > 0 {
		fmt.Fprintln(w, "Suggested (written as commented sections; uncomment to enable)")
		writeTier(w, r.suggested)
	}
	fmt.Fprintln(w, "Not inferred (needs a decision only you can make)")
	writeTier(w, r.notInferred)
	fmt.Fprintln(w, "Next")
	fmt.Fprintln(w, "  review the config, then: memvet check")
}

func writeTier(w io.Writer, lines []tierLine) {
	for _, l := range lines {
		fmt.Fprintf(w, "  %-13s %s\n", l.rule, l.why)
	}
}

// evidence is what one read-only pass over the repository turned up.
type evidence struct {
	indexFiles []string // existing top-level index files, candidate [pointers] sources
	mdRoots    []string // top-level dirs that contain markdown, candidate [pointers] roots
	dsStore    bool     // a .DS_Store exists somewhere in the tree
	tmpFiles   bool     // a *.tmp exists somewhere in the tree

	// Suggestion evidence. The list is closed by ruling (D-143 §3): a
	// decisions log by name, and two or more instruction files at the root.
	decisionsLog     string   // first decisions.md / decision-log.md under an md root
	instructionFiles []string // of CLAUDE.md, AGENTS.md, GEMINI.md, the ones that exist

	// flatNotes counts markdown files at the root other than MEMORY.md. A
	// MEMORY.md beside two or more of them, with no md root at all, is a
	// flat memory folder (Claude Code's auto-memory has that shape), and
	// the evidence for pointers with the sibling root "." (v0.11).
	flatNotes int
}

// indexCandidates are the files agent runtimes conventionally read as memory
// indexes. Only ones that actually exist are configured.
var indexCandidates = []string{
	"MEMORY.md", "CLAUDE.md", "AGENTS.md",
	// v0.11: the other per-runtime instruction files. Each is plain text
	// with repo paths in it, so a dead reference in one is the same defect.
	"GEMINI.md", "COPILOT.md", ".github/copilot-instructions.md", ".cursorrules", "CONVENTIONS.md",
}

// instructionCandidates are the per-runtime instruction files. Two or more
// present at the root is the evidence for suggesting [mirrors].
var instructionCandidates = []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"}

func inspect(root string) evidence {
	var ev evidence

	for _, name := range indexCandidates {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			ev.indexFiles = append(ev.indexFiles, name)
		}
	}

	for _, name := range instructionCandidates {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			ev.instructionFiles = append(ev.instructionFiles, name)
		}
	}

	roots := map[string]bool{}
	// WalkDir does not follow symlinked directories, so a symlink cannot pull
	// content from outside the tree into the evidence.
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corners are check's problem, not init's
		}
		if d.IsDir() {
			if p != root && (d.Name() == ".git" || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case d.Name() == ".DS_Store":
			ev.dsStore = true
		case strings.HasSuffix(d.Name(), ".tmp"):
			ev.tmpFiles = true
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !strings.Contains(rel, "/") && rel != "MEMORY.md" {
				ev.flatNotes++
			}
			if i := strings.Index(rel, "/"); i > 0 {
				roots[rel[:i]] = true
				base := strings.ToLower(d.Name())
				if ev.decisionsLog == "" && (base == "decisions.md" || base == "decision-log.md") {
					ev.decisionsLog = rel
				}
			}
		}
		return nil
	})
	ev.mdRoots = make([]string, 0, len(roots))
	for r := range roots {
		ev.mdRoots = append(ev.mdRoots, r)
	}
	sort.Strings(ev.mdRoots)
	return ev
}

// flatMemory is the evidence for a flat memory folder: MEMORY.md at the
// root and no markdown folder to name as a root. The note count is not a
// condition (v0.11.1): a folder whose only note was deleted is the case the
// rule exists for, and MEMORY.md alone with a dead link must still be RED.
func (ev evidence) flatMemory() bool {
	if len(ev.mdRoots) > 0 {
		return false
	}
	for _, f := range ev.indexFiles {
		if f == "MEMORY.md" {
			return true
		}
	}
	return false
}

// renderConfig produces the generated file and the report that explains it.
// The output must always pass config.Load — that property is pinned by test.
// The file carries only what the evidence supports: enabled sections, and
// commented sections for the suggested rules. The rule reference lives in
// the docs, not in every generated config.
func renderConfig(ev evidence) (string, initReport) {
	var b strings.Builder
	var rep initReport

	b.WriteString(`# .memvet.toml — generated by memvet init from an inspection of this
# repository. Only rules with observed evidence are enabled. A section's
# presence is what enables its rule. Rules and recipes:
# https://github.com/frankbesch/memvet/blob/main/docs/recipes.md

`)

	switch {
	case len(ev.indexFiles) > 0 && len(ev.mdRoots) > 0:
		rep.enabled = append(rep.enabled, tierLine{"pointers",
			strings.Join(ev.indexFiles, ", ") + " -> roots " + strings.Join(ev.mdRoots, ", ")})
		b.WriteString("# Repo-path references in these files must resolve to files that exist.\n")
		b.WriteString("[pointers]\n")
		b.WriteString("files = " + tomlList(ev.indexFiles) + "\n")
		b.WriteString("# A reference is checked only when its first path segment is a root here.\n")
		b.WriteString("roots = " + tomlList(ev.mdRoots) + "\n\n")
	case ev.flatMemory():
		rep.enabled = append(rep.enabled, tierLine{"pointers",
			fmt.Sprintf("MEMORY.md beside %d notes, no folder -> roots \".\" (sibling links)", ev.flatNotes)})
		b.WriteString("# Links in MEMORY.md must resolve to the notes beside it.\n")
		b.WriteString("[pointers]\n")
		b.WriteString("files = [\"MEMORY.md\"]\n")
		b.WriteString("# \".\" checks slash-less link destinations against the file's own folder.\n")
		b.WriteString("roots = [\".\"]\n\n")
	default:
		rep.notInferred = append(rep.notInferred, tierLine{"pointers", "no index file (MEMORY.md, CLAUDE.md, AGENTS.md, ...) next to a folder of markdown"})
	}

	if ev.dsStore || ev.tmpFiles {
		var globs, seen []string
		if ev.dsStore {
			globs = append(globs, ".DS_Store")
			seen = append(seen, ".DS_Store")
		}
		if ev.tmpFiles {
			globs = append(globs, "*.tmp")
			seen = append(seen, "*.tmp")
		}
		rep.enabled = append(rep.enabled, tierLine{"junk", strings.Join(seen, " and ") + " seen in the tree"})
		b.WriteString("# Files matching these globs were found in the tree and should not be here.\n")
		b.WriteString("[junk]\n")
		b.WriteString("globs = " + tomlList(globs) + "\n\n")
	} else {
		rep.notInferred = append(rep.notInferred, tierLine{"junk", "no .DS_Store or *.tmp seen"})
	}

	if ev.decisionsLog != "" {
		rep.suggested = append(rep.suggested, tierLine{"append_only", ev.decisionsLog + " exists"})
		b.WriteString("# Suggested: a decisions log by name. Uncomment if it may only grow.\n")
		b.WriteString("# [append_only]\n")
		b.WriteString("# files = " + tomlList([]string{ev.decisionsLog}) + "\n\n")
	}

	if len(ev.instructionFiles) >= 2 {
		rep.suggested = append(rep.suggested, tierLine{"mirrors", strings.Join(ev.instructionFiles, " and ") + " both present"})
		b.WriteString("# Suggested: two instruction files at the root. Uncomment if they must stay identical.\n")
		b.WriteString("# [mirrors]\n")
		b.WriteString("# pairs = [" + tomlList(ev.instructionFiles[:2]) + "]\n\n")
	}

	// Policy-dependent rules are never guessed. Each line says what the
	// decision is, so the reader knows it is theirs.
	if ev.decisionsLog == "" {
		rep.notInferred = append(rep.notInferred, tierLine{"append_only", "no decisions.md found; which log may only grow is your call"})
	}
	if len(ev.instructionFiles) < 2 {
		rep.notInferred = append(rep.notInferred, tierLine{"mirrors", "fewer than two of CLAUDE.md, AGENTS.md, GEMINI.md at the root"})
	}
	rep.notInferred = append(rep.notInferred,
		tierLine{"blocks", "the ownership markers are a convention you pick"},
		tierLine{"human_brief", "authorship is a policy choice"},
		tierLine{"tokens", "a budget is a choice"},
		tierLine{"ids", "the id shape is a convention you pick"},
		tierLine{"stamps", "a maximum age is a choice"},
		tierLine{"secrets", "enable deliberately; it scans every file you list"},
	)

	return strings.TrimRight(b.String(), "\n") + "\n", rep
}

// tomlList renders a TOML array of strings. %q quoting is a valid TOML basic
// string for repository-relative paths.
func tomlList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
