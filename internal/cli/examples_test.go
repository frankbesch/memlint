package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The directories under examples/ are the recipes in docs/recipes.md and the
// output quoted in README.md. They are documentation that runs: each one is
// held to the result its recipe states, so a recipe cannot drift from what
// memvet actually does.

func example(name string) string {
	return filepath.Join("..", "..", "examples", name)
}

func TestExamplesMatchTheirRecipes(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		summary string // prefix of the last stdout line
	}{
		{"minimal", 0, "memvet: clean (3 rules, 4 files checked"},
		{"shared-instructions", 0, "memvet: clean (2 rules, 2 files checked"},
		{"broken", 1, "memvet: 1 red, 1 yellow"},
	}
	for _, c := range cases {
		got := run(t, nil, "check", "--no-color", example(c.name))
		last := lastLine(got.stdout)
		if got.code != c.code || !strings.HasPrefix(last, c.summary) {
			t.Errorf("examples/%s: exit %d, last line %q; want exit %d and prefix %q\nstdout:\n%s\nstderr:\n%s",
				c.name, got.code, last, c.code, c.summary, got.stdout, got.stderr)
		}
	}
}

// decision-log needs a git baseline, so it is checked as a committed repo.
// In this repository's own tree it is committed too, but a test must not
// depend on the working tree's git state.
func TestDecisionLogExampleIsCleanOnceCommitted(t *testing.T) {
	dir := copyTree(t, example("decision-log"))
	git(t, dir, "init", "-q")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "log")

	got := run(t, nil, "check", "--no-color", dir)
	if got.code != 0 || !strings.HasPrefix(lastLine(got.stdout), "memvet: clean (2 rules, 1 file checked") {
		t.Fatalf("committed decision-log: exit %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}

	// Rewriting a past entry is the failure the recipe exists to catch.
	logPath := filepath.Join(dir, "memory", "decisions.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(content), "D-001 | 2026-08-20 | Move billing", "D-001 | 2026-08-20 | Keep billing", 1)
	if err := os.WriteFile(logPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	got = run(t, nil, "check", "--no-color", dir)
	if got.code != 1 || !strings.Contains(got.stdout, "[append_only/rewritten]") {
		t.Fatalf("rewritten entry not caught: exit %d\nstdout:\n%s", got.code, got.stdout)
	}
}

// README.md quotes the output of `memvet check examples/broken`. The quote
// must be the real thing, byte for byte, or the landing page lies.
func TestReadmeOutputMatchesExample(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)<!-- examples/broken output:[^\n]*-->\n```text\n(.*?)```")
	m := re.FindSubmatch(readme)
	if m == nil {
		t.Fatal("README.md has no marked examples/broken output block")
	}
	got := run(t, nil, "check", "--no-color", example("broken"))
	if string(m[1]) != got.stdout {
		t.Errorf("README output block is stale.\nREADME:\n%s\nreal run:\n%s", m[1], got.stdout)
	}
}

// The first-run journey from the README: init on a small repo, check clean,
// break a reference, check red.
func TestFirstRunJourney(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("MEMORY.md", "# Memory\n\n- [acme](memory/acme.md)\n- [prefs](memory/preferences.md)\n")
	write("memory/acme.md", "# acme\n\nstatus: staging\n")
	write("memory/preferences.md", "# prefs\n\nanswer first\n")

	if got := run(t, nil, "init", dir); got.code != 0 {
		t.Fatalf("init: exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
	if got := run(t, nil, "check", "--no-color", dir); got.code != 0 || !strings.HasPrefix(got.stdout, "memvet: clean") {
		t.Fatalf("first check should be clean: exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
	if err := os.Remove(filepath.Join(dir, "memory", "preferences.md")); err != nil {
		t.Fatal(err)
	}
	got := run(t, nil, "check", "--no-color", dir)
	if got.code != 1 || !strings.Contains(got.stdout, "[pointers/dead-ref]") {
		t.Fatalf("deleted note should be a dead reference: exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
