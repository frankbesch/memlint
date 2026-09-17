package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC.md v0.11 addendum, parts 1-4. Each test is a declared gate.

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Part 1, G1: no config runs the inferred config, YELLOW first, RED after,
// and writes nothing.
func TestNoConfigRunsInferredConfigAndSaysSo(t *testing.T) {
	dir := tree(t, map[string]string{
		"MEMORY.md":   "# M\n- [a](memory/a.md)\n- [gone](memory/gone.md)\n",
		"memory/a.md": "a\n",
	})
	got := run(t, nil, "check", "--no-color", dir)
	lines := strings.Split(strings.TrimRight(got.stdout, "\n"), "\n")
	if got.code != 1 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q\n%s", got.code, got.stderr, got.stdout)
	}
	if !strings.HasPrefix(lines[0], "config ") || !strings.Contains(lines[0], "YELLOW  .memvet.toml") || !strings.Contains(lines[0], "[config/inferred]") || !strings.Contains(lines[0], "(pointers)") {
		t.Errorf("first line must be the config/inferred YELLOW naming the rules, got %q", lines[0])
	}
	if !strings.Contains(got.stdout, "[pointers/dead-ref]") {
		t.Errorf("the inferred pointers rule must have run:\n%s", got.stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, ".memvet.toml")); !os.IsNotExist(err) {
		t.Errorf("check must not write a config (stat err: %v)", err)
	}
}

// Part 1, G2: the YELLOW alone fails under --strict and passes without it.
func TestInferredConfigFailsOnlyUnderStrict(t *testing.T) {
	dir := tree(t, map[string]string{
		"MEMORY.md":   "# M\n- [a](memory/a.md)\n",
		"memory/a.md": "a\n",
	})
	if got := run(t, nil, "check", "--no-color", dir); got.code != 0 || !strings.Contains(got.stdout, "0 red, 1 yellow") {
		t.Errorf("without --strict: exit %d\n%s", got.code, got.stdout)
	}
	if got := run(t, nil, "check", "--no-color", "--strict", dir); got.code != 1 {
		t.Errorf("with --strict: exit %d, want 1\n%s", got.code, got.stdout)
	}
}

// Part 1, G3: an empty tree infers nothing and says so.
func TestNoConfigOnEmptyTreeSaysNothingRan(t *testing.T) {
	got := run(t, nil, "check", "--no-color", t.TempDir())
	if got.code != 0 || !strings.Contains(got.stdout, "nothing to infer, so no rules ran") || !strings.Contains(got.stdout, "0 red, 1 yellow") {
		t.Errorf("exit %d\n%s", got.code, got.stdout)
	}
}

// Part 1, G4: an invalid config is still a startup error; only absence infers.
func TestMalformedConfigIsStillAStartupError(t *testing.T) {
	dir := tree(t, map[string]string{".memvet.toml": "[bogus]\nx = 1\n"})
	got := run(t, nil, "check", dir)
	if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, "unknown key") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

// Part 1, G5: the config finding travels through json and github like any other.
func TestInferredFindingInEveryFormat(t *testing.T) {
	dir := tree(t, map[string]string{"MEMORY.md": "# M\n- [a](memory/a.md)\n", "memory/a.md": "a\n"})
	got := run(t, nil, "check", "--format", "json", dir)
	var v struct {
		Findings []struct {
			Code   string `json:"code"`
			DocURL string `json:"doc_url"`
		}
		Summary struct{ Yellow int }
	}
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil || len(v.Findings) != 1 || v.Findings[0].Code != "config/inferred" || v.Summary.Yellow != 1 {
		t.Errorf("json: err %v, %+v\n%s", err, v, got.stdout)
	}
	got = run(t, nil, "check", "--format", "github", dir)
	if !strings.Contains(got.stdout, "::warning") || !strings.Contains(got.stdout, "config/inferred") {
		t.Errorf("github format:\n%s", got.stdout)
	}
}

// Part 2, G1: the other runtimes' instruction files are index candidates.
func TestInitDiscoversOtherRuntimeInstructionFiles(t *testing.T) {
	dir := tree(t, map[string]string{
		".github/copilot-instructions.md": "see docs/a.md\n",
		".cursorrules":                    "read docs/a.md first\n",
		"docs/a.md":                       "a\n",
	})
	got := run(t, nil, "init", "--dry-run", dir)
	if got.code != 0 {
		t.Fatalf("exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "pointers      .github/copilot-instructions.md, .cursorrules -> roots docs") {
		t.Errorf("report should enable pointers on both files:\n%s", got.stderr)
	}
	if !strings.Contains(got.stdout, `files = [".github/copilot-instructions.md", ".cursorrules"]`) {
		t.Errorf("config:\n%s", got.stdout)
	}
}

// Part 3, G1/G2/G4: a flat memory folder is checked through sibling links,
// prose stays untouched, and init infers the "." root for it.
func TestFlatMemoryFolder(t *testing.T) {
	dir := tree(t, map[string]string{
		"MEMORY.md": "# M\n- [here](here.md) — hook\n- [gone](gone.md) — hook\nsee a.md for context\n",
		"here.md":   "# Here\n",
		"other.md":  "x\n",
	})
	got := run(t, nil, "init", "--dry-run", dir)
	if got.code != 0 || !strings.Contains(got.stdout, `roots = ["."]`) || !strings.Contains(got.stderr, "sibling links") {
		t.Fatalf("init should infer the sibling root: exit %d\n%s%s", got.code, got.stdout, got.stderr)
	}
	got = run(t, nil, "check", "--no-color", dir)
	if got.code != 1 || strings.Count(got.stdout, "[pointers/dead-ref]") != 1 || !strings.Contains(got.stdout, "MEMORY.md:3   dead reference: gone.md") {
		t.Errorf("exactly one dead sibling link expected:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "a.md") {
		t.Errorf("a bare word in prose must never be a sibling reference:\n%s", got.stdout)
	}
}

// Part 4 support: the composite action and the pre-commit hook are present
// and name the real command. The end-to-end run is the CI job "action".
func TestActionAndPreCommitFilesExist(t *testing.T) {
	for file, want := range map[string]string{
		"action.yml":             "memvet check --format github",
		".pre-commit-hooks.yaml": "entry: memvet check --changed",
	} {
		b, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("%s should contain %q", file, want)
		}
	}
}

// v0.11.1: a flat memory folder is inferred from MEMORY.md alone. A one-note
// folder (the docs/recipes.md one-liner on a young auto-memory folder) and a
// folder whose only note was deleted both run pointers and report the dead link.
func TestFlatMemoryInferredWithoutSiblingThreshold(t *testing.T) {
	one := tree(t, map[string]string{
		"MEMORY.md": "# M\n- [Keep](keep.md)\n- [Gone](gone.md)\nsee a.md for context\n",
		"keep.md":   "keep\n",
	})
	got := run(t, nil, "check", "--no-color", one)
	if got.code != 1 || !strings.Contains(got.stdout, "ran the inferred config (pointers)") || !strings.Contains(got.stdout, "gone.md does not exist") || strings.Contains(got.stdout, "a.md") {
		t.Errorf("one note: exit %d\n%s", got.code, got.stdout)
	}
	alone := tree(t, map[string]string{"MEMORY.md": "# M\n- [Gone](gone.md)\n"})
	got = run(t, nil, "check", "--no-color", alone)
	if got.code != 1 || !strings.Contains(got.stdout, "gone.md does not exist") {
		t.Errorf("MEMORY.md alone: exit %d\n%s", got.code, got.stdout)
	}
	got = run(t, nil, "init", "--dry-run", alone)
	if got.code != 0 || !strings.Contains(got.stdout, "roots = [\".\"]") {
		t.Errorf("init on MEMORY.md alone: exit %d\n%s", got.code, got.stdout)
	}
}
