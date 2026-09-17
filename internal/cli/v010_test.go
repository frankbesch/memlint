package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// SPEC.md v0.10 addendum, parts 1-4 (D-143). Each test is a declared gate.

// Part 1, G1/G2: a flag is honored before or after the path, byte for byte.
func TestFlagsAnywhereAroundThePath(t *testing.T) {
	for _, tc := range [][2][]string{
		{{"check", "--no-color", "--strict", fixture("fixture-clean")}, {"check", fixture("fixture-clean"), "--strict", "--no-color"}},
		{{"check", "--format", "json", fixture("fixture-broken")}, {"check", fixture("fixture-broken"), "--format=json"}},
		{{"check", "--no-color", "--expect-tree", "000000000000", fixture("fixture-clean")}, {"check", fixture("fixture-clean"), "--expect-tree", "000000000000", "--no-color"}},
	} {
		a, b := run(t, nil, tc[0]...), run(t, nil, tc[1]...)
		if a.code != b.code || a.stdout != b.stdout {
			t.Errorf("%v (exit %d) and %v (exit %d) must match:\n%s\n---\n%s", tc[0], a.code, tc[1], b.code, a.stdout, b.stdout)
		}
	}
	got := run(t, nil, "check", fixture("fixture-broken"), "--format=json")
	var v map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil || got.code != 1 {
		t.Errorf("--format=json after the path: exit %d, parse error %v", got.code, err)
	}
}

// Part 1, G3: what is refused is refused loudly, with the command's help.
func TestBadArgumentsAreLoud(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"check", "--strict", "a", "b"}, `unexpected argument "b"`},
		{[]string{"check", ".", "--bogus"}, `unknown flag "--bogus"`},
		{[]string{"check", "--base"}, "needs a value"},
		{[]string{"init", "--force", "."}, `unknown flag "--force"`},
		{[]string{"fingerprint", ".", "--strict"}, `unknown flag "--strict"`},
	}
	for _, c := range cases {
		got := run(t, nil, c.args...)
		if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, c.want) {
			t.Errorf("%v: exit %d, stdout %q, stderr %q; want exit 2 and %q on stderr", c.args, got.code, got.stdout, got.stderr, c.want)
		}
		if !strings.Contains(got.stderr, "memvet "+c.args[0]+" ") {
			t.Errorf("%v: stderr should carry the %s help, got:\n%s", c.args, c.args[0], got.stderr)
		}
	}
}

// Part 2, G1: the top-level help fits one screen and does not leak flags.
func TestTopLevelHelpIsOneScreen(t *testing.T) {
	got := run(t, nil, "--help")
	lines := strings.Count(got.stdout, "\n")
	if got.code != 0 || lines > 24 {
		t.Errorf("--help: exit %d, %d lines (want <= 24)", got.code, lines)
	}
	for _, want := range []string{"check", "init", "fingerprint"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("--help should name %s", want)
		}
	}
	if strings.Contains(got.stdout, "--expect-tree") {
		t.Error("--help should not carry check's flags")
	}
}

// Part 2, G2: every flag in docs/cli.md's table appears in check --help.
func TestCheckHelpCoversDocumentedFlags(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?m)^\\| `(--[a-z-]+)")
	flags := re.FindAllStringSubmatch(string(doc), -1)
	if len(flags) < 5 {
		t.Fatalf("docs/cli.md flag table not found (%d rows)", len(flags))
	}
	help := run(t, nil, "check", "--help").stdout
	for _, m := range flags {
		if !strings.Contains(help, m[1]) {
			t.Errorf("check --help is missing documented flag %s", m[1])
		}
	}
}

// Part 2, G3/G4: each command owns its help, and usage errors show the right one.
func TestPerCommandHelp(t *testing.T) {
	if got := run(t, nil, "init", "--help"); got.code != 0 || !strings.Contains(got.stdout, "never overwrite") {
		t.Errorf("init --help: exit %d\n%s", got.code, got.stdout)
	}
	if got := run(t, nil, "fingerprint", "--help"); got.code != 0 || !strings.Contains(got.stdout, "--expect-tree") {
		t.Errorf("fingerprint --help: exit %d\n%s", got.code, got.stdout)
	}
	if a, b := run(t, nil, "help", "check").stdout, run(t, nil, "check", "-h").stdout; a == "" || a != b {
		t.Error("memvet help check must equal memvet check -h")
	}
	got := run(t, nil, "check", "--bogus")
	if !strings.Contains(got.stderr, "memvet check [flags]") || strings.Contains(got.stderr, "memvet init [flags]") {
		t.Errorf("check usage error should show check's help only:\n%s", got.stderr)
	}
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

var suggestionTree = map[string]string{
	"MEMORY.md":           "- [notes](memory/notes.md)\n",
	"memory/notes.md":     "notes\n",
	"memory/decisions.md": "D-001 | 2026-09-11 | first\n",
	"CLAUDE.md":           "instructions\n",
	"AGENTS.md":           "instructions\n",
}

// Part 3, G1/G3: enabled on evidence, suggested on the two closed heuristics,
// nothing else commented in, and the file stays short.
func TestInitReportsTiers(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, suggestionTree)
	got := run(t, nil, "init", dir)
	if got.code != 0 {
		t.Fatalf("init: exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
	out := got.stdout
	enabledIdx, suggestedIdx, notIdx := strings.Index(out, "Enabled"), strings.Index(out, "Suggested"), strings.Index(out, "Not inferred")
	if enabledIdx < 0 || suggestedIdx < enabledIdx || notIdx < suggestedIdx {
		t.Fatalf("report tiers out of order or missing:\n%s", out)
	}
	if !strings.Contains(out[enabledIdx:suggestedIdx], "pointers") {
		t.Errorf("pointers should be Enabled:\n%s", out)
	}
	for _, r := range []string{"append_only", "mirrors"} {
		if !strings.Contains(out[suggestedIdx:notIdx], r) {
			t.Errorf("%s should be Suggested:\n%s", r, out)
		}
	}
	for _, r := range []string{"human_brief", "tokens", "blocks", "ids", "stamps", "secrets"} {
		if !strings.Contains(out[notIdx:], r) {
			t.Errorf("%s should be Not inferred:\n%s", r, out)
		}
	}

	cfg, err := os.ReadFile(filepath.Join(dir, ".memvet.toml"))
	if err != nil {
		t.Fatal(err)
	}
	commented := regexp.MustCompile(`(?m)^# \[([a-z_]+)\]`).FindAllStringSubmatch(string(cfg), -1)
	var names []string
	for _, m := range commented {
		names = append(names, m[1])
	}
	if strings.Join(names, ",") != "append_only,mirrors" {
		t.Errorf("commented sections should be exactly append_only,mirrors; got %v", names)
	}
	if n := strings.Count(string(cfg), "\n"); n > 30 {
		t.Errorf("generated config is %d lines, want <= 30", n)
	}
	if check := run(t, nil, "check", "--no-color", dir); check.code != 0 {
		t.Errorf("generated config must load and run clean: exit %d\n%s%s", check.code, check.stdout, check.stderr)
	}
}

// Part 3, G2: an empty repo enables nothing and says so.
func TestInitEmptyRepoListsEverythingAsNotInferred(t *testing.T) {
	dir := t.TempDir()
	got := run(t, nil, "init", dir)
	if got.code != 0 || strings.Contains(got.stdout, "Enabled\n") || strings.Contains(got.stdout, "Suggested") {
		t.Fatalf("empty repo: exit %d, unexpected tiers:\n%s", got.code, got.stdout)
	}
	for _, r := range []string{"pointers", "junk", "append_only", "mirrors", "blocks", "human_brief", "tokens", "ids", "stamps", "secrets"} {
		if !strings.Contains(got.stdout, "  "+r+" ") {
			t.Errorf("%s should be listed as Not inferred", r)
		}
	}
	if !strings.Contains(got.stdout, "no rules enabled") {
		t.Error("report should say what check will report")
	}
}

// Part 4, G1-G3: --dry-run prints the config and writes nothing, even when a
// config already exists; --force does not exist.
func TestInitDryRun(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, suggestionTree)

	dry := run(t, nil, "init", "--dry-run", dir)
	if dry.code != 0 {
		t.Fatalf("dry-run: exit %d\n%s", dry.code, dry.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".memvet.toml")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create .memvet.toml")
	}
	if !strings.Contains(dry.stderr, "nothing written") || strings.Contains(dry.stdout, "Enabled") {
		t.Errorf("report belongs on stderr, config on stdout:\nstdout:\n%s\nstderr:\n%s", dry.stdout, dry.stderr)
	}

	if got := run(t, nil, "init", dir); got.code != 0 {
		t.Fatal(got.stderr)
	}
	written, err := os.ReadFile(filepath.Join(dir, ".memvet.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != dry.stdout {
		t.Errorf("dry-run stdout must equal the file init writes:\n%s\n---\n%s", dry.stdout, written)
	}

	again := run(t, nil, "init", dir, "--dry-run")
	after, _ := os.ReadFile(filepath.Join(dir, ".memvet.toml"))
	if again.code != 0 || string(after) != string(written) || again.stdout != dry.stdout {
		t.Errorf("dry-run on a configured repo must succeed and leave the file alone (exit %d)", again.code)
	}
}
