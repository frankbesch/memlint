package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// binPath is a memlint binary built once for the whole package. The CLI
// contract is about exit codes and stream routing, which only a real process
// can demonstrate.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "memlint-cli-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, "creating temp dir:", err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "memlint")

	build := exec.Command("go", "build", "-o", binPath, "github.com/frankbesch/memlint")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building memlint: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running memlint %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func fixture(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"clean repository", []string{"check", fixture("fixture-clean")}, 0},
		{"red findings", []string{"check", fixture("fixture-broken")}, 1},
		{"yellow findings are not a failure", []string{"check", fixture("fixture-yellow")}, 0},
		{"yellow findings fail under strict", []string{"check", "--strict", fixture("fixture-yellow")}, 1},
		{"red findings under strict", []string{"check", "--strict", fixture("fixture-broken")}, 1},
		{"invalid config", []string{"check", fixture("fixture-badconfig")}, 2},
		{"no command", nil, 2},
		{"unknown command", []string{"lint"}, 2},
		{"unknown flag", []string{"check", "--fix"}, 2},
		{"unknown format", []string{"check", "--format", "html", fixture("fixture-clean")}, 2},
		{"flag after path", []string{"check", fixture("fixture-yellow"), "--strict"}, 2},
		{"target does not exist", []string{"check", "no/such/dir"}, 2},
		{"help", []string{"-h"}, 0},
		{"check help", []string{"check", "-h"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, nil, tt.args...)
			if got.code != tt.want {
				t.Errorf("memlint %v exited %d, want %d\nstdout:\n%s\nstderr:\n%s",
					tt.args, got.code, tt.want, got.stdout, got.stderr)
			}
		})
	}
}

func TestVersionFlag(t *testing.T) {
	got := run(t, nil, "--version")
	if got.code != 0 {
		t.Errorf("exited %d, want 0\nstderr:\n%s", got.code, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("--version must leave stderr empty, got: %q", got.stderr)
	}
	// The exact version varies by how the binary was built: a VCS-stamped
	// pseudo-version here, a module version under `go install`, "dev" when no
	// build info survives. The contract is the shape, not the value.
	lines := strings.Split(strings.TrimRight(got.stdout, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "memlint ") {
		t.Errorf("want one line of the form \"memlint <version>\", got: %q", got.stdout)
	}
	if lines[0] == "memlint " {
		t.Errorf("version must not be empty, got: %q", lines[0])
	}
}

// Release binaries get their version through -ldflags -X. This build pins the
// symbol name that .goreleaser.yaml points at: renaming cli.version breaks
// this test before it silently breaks release builds.
func TestVersionInjection(t *testing.T) {
	injected := filepath.Join(t.TempDir(), "memlint")
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/frankbesch/memlint/internal/cli.version=v9.9.9-test",
		"-o", injected, "github.com/frankbesch/memlint")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building with -ldflags: %v\n%s", err, out)
	}
	out, err := exec.Command(injected, "--version").Output()
	if err != nil {
		t.Fatalf("running --version: %v", err)
	}
	if got := strings.TrimRight(string(out), "\n"); got != "memlint v9.9.9-test" {
		t.Errorf("got %q, want %q", got, "memlint v9.9.9-test")
	}
}

// A flag written after the path would otherwise be swallowed silently by the
// standard flag package, so --strict would appear to work while doing nothing.
func TestFlagAfterPathIsRefusedNotIgnored(t *testing.T) {
	got := run(t, nil, "check", fixture("fixture-yellow"), "--strict")
	if !strings.Contains(got.stderr, "flags must precede the path") {
		t.Errorf("stderr should explain the argument order:\n%s", got.stderr)
	}
}

func TestCleanRunIsOneGreenLine(t *testing.T) {
	got := run(t, nil, "check", fixture("fixture-clean"))
	lines := strings.Split(strings.TrimRight(got.stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("a clean run must print exactly one line, got %d:\n%s", len(lines), got.stdout)
	}
	if !strings.HasPrefix(lines[0], "memlint: clean") {
		t.Errorf("got %q", lines[0])
	}
}

// An empty config is not an error, but its output must not read as though
// anything was verified.
func TestNoRulesEnabledSaysSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".memlint.toml"), []byte("# nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := run(t, nil, "check", dir)
	if got.code != 0 {
		t.Errorf("exited %d, want 0", got.code)
	}
	if !strings.Contains(got.stdout, "no rules enabled") {
		t.Errorf("output must disclose that nothing ran, got: %q", got.stdout)
	}
}

func TestMissingConfigIsAStartupError(t *testing.T) {
	got := run(t, nil, "check", t.TempDir())
	if got.code != 2 {
		t.Errorf("exited %d, want 2", got.code)
	}
	if got.stdout != "" {
		t.Errorf("startup errors must not write to stdout, got: %q", got.stdout)
	}
	if !strings.Contains(got.stderr, ".memlint.toml") {
		t.Errorf("stderr should name the missing file, got: %q", got.stderr)
	}
}

func TestFindingsGoToStdoutErrorsToStderr(t *testing.T) {
	got := run(t, nil, "check", fixture("fixture-broken"))
	if !strings.Contains(got.stdout, "dead reference") {
		t.Errorf("findings belong on stdout, got:\n%s", got.stdout)
	}
	if got.stderr != "" {
		t.Errorf("a normal run must leave stderr empty, got:\n%s", got.stderr)
	}
}

func TestJSONFormat(t *testing.T) {
	got := run(t, nil, "check", "--format", "json", fixture("fixture-broken"))
	if got.code != 1 {
		t.Fatalf("exited %d, want 1\n%s", got.code, got.stderr)
	}

	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Findings      []struct {
			Rule        string `json:"rule"`
			Code        string `json:"code"`
			Severity    string `json:"severity"`
			Path        string `json:"path"`
			RelatedPath string `json:"related_path"`
			Line        int    `json:"line"`
			Message     string `json:"message"`
		} `json:"findings"`
		Summary struct {
			Red    int `json:"red"`
			Yellow int `json:"yellow"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, got.stdout)
	}
	if doc.SchemaVersion != 1 {
		t.Errorf("got schema_version %d, want 1", doc.SchemaVersion)
	}
	if doc.Summary.Red != 7 || doc.Summary.Yellow != 3 {
		t.Errorf("got %d red / %d yellow, want 7 / 3", doc.Summary.Red, doc.Summary.Yellow)
	}
	if len(doc.Findings) != 10 {
		t.Errorf("got %d findings, want 10", len(doc.Findings))
	}
	// Codes are the stable machine identity of a finding. Every finding must
	// carry one, in "<rule>/<kind>" form.
	codeRe := regexp.MustCompile(`^[a-z_]+/[a-z-]+$`)
	seen := map[string]bool{}
	for _, f := range doc.Findings {
		if !codeRe.MatchString(f.Code) {
			t.Errorf("finding %q has malformed code %q", f.Message, f.Code)
		}
		seen[f.Code] = true
	}
	for _, want := range []string{"blocks/unterminated", "blocks/duplicate-start",
		"mirrors/differ", "mirrors/one-sided", "pointers/dead-ref",
		"junk/match", "tokens/over-budget", "tokens/no-match"} {
		if !seen[want] {
			t.Errorf("fixture-broken should produce code %q, got %v", want, seen)
		}
	}
	if strings.Contains(got.stdout, "\033") {
		t.Error("JSON output must never contain ANSI escapes")
	}
	for _, f := range doc.Findings {
		if filepath.IsAbs(f.Path) {
			t.Errorf("paths must be root-relative, got %q", f.Path)
		}
	}
}

func TestGitHubFormat(t *testing.T) {
	got := run(t, nil, "check", "--format", "github", fixture("fixture-broken"))
	if got.code != 1 {
		t.Fatalf("exited %d, want 1\n%s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "::error file=memory/index.md,line=7,title=memlint pointers/dead-ref::") {
		t.Errorf("missing RED annotation:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "::warning file=") {
		t.Errorf("missing YELLOW annotation:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "\033") {
		t.Error("github output must never contain ANSI escapes")
	}
	if !strings.Contains(got.stdout, "memlint: 7 red, 3 yellow") {
		t.Errorf("missing summary line:\n%s", got.stdout)
	}
}

func TestJSONCleanRunHasEmptyFindingsList(t *testing.T) {
	got := run(t, nil, "check", "--format", "json", fixture("fixture-clean"))
	if !strings.Contains(got.stdout, `"findings": []`) {
		t.Errorf("clean JSON must carry an empty list, not null:\n%s", got.stdout)
	}
}

// Output order must not depend on filesystem walk order.
func TestOutputIsDeterministic(t *testing.T) {
	first := run(t, nil, "check", "--format", "json", fixture("fixture-broken"))
	for i := 0; i < 3; i++ {
		again := run(t, nil, "check", "--format", "json", fixture("fixture-broken"))
		if again.stdout != first.stdout {
			t.Fatalf("run %d differed from the first:\n%s\n---\n%s", i+2, first.stdout, again.stdout)
		}
	}
}

func TestColorIsSuppressed(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		args []string
	}{
		{"--no-color", nil, []string{"check", "--no-color", fixture("fixture-broken")}},
		{"NO_COLOR", []string{"NO_COLOR=1"}, []string{"check", fixture("fixture-broken")}},
		{"not a terminal", nil, []string{"check", fixture("fixture-broken")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, tc.env, tc.args...)
			if strings.Contains(got.stdout, "\033") {
				t.Errorf("output contains ANSI escapes:\n%q", got.stdout)
			}
		})
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	got := run(t, nil, "-h")
	if !strings.Contains(got.stdout, "memlint check") {
		t.Errorf("help belongs on stdout, got stdout=%q stderr=%q", got.stdout, got.stderr)
	}
}

func TestUsageErrorsGoToStderr(t *testing.T) {
	got := run(t, nil, "lint")
	if got.stdout != "" {
		t.Errorf("usage errors must not write to stdout, got: %q", got.stdout)
	}
	if !strings.Contains(got.stderr, "unknown command") {
		t.Errorf("stderr should name the problem, got: %q", got.stderr)
	}
}

// init on a repo with evidence must produce a config that check can run
// immediately — and that surfaces the evidence it was built from.
func TestInitGeneratesWorkingConfig(t *testing.T) {
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
	write("MEMORY.md", "- [notes](memory/notes.md)\n- [gone](memory/missing.md)\n")
	write("memory/notes.md", "notes\n")
	write("memory/.DS_Store", "junk")

	got := run(t, nil, "init", dir)
	if got.code != 0 {
		t.Fatalf("init exited %d\nstderr:\n%s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "2 rules enabled") {
		t.Errorf("expected 2 evidence-based rules, got: %q", got.stdout)
	}

	// The generated config must load and run, and catch the planted defects:
	// a dead reference (RED) and the .DS_Store that justified [junk] (YELLOW).
	check := run(t, nil, "check", "--no-color", dir)
	if check.code != 1 {
		t.Fatalf("check on generated config exited %d, want 1\nstdout:\n%s\nstderr:\n%s",
			check.code, check.stdout, check.stderr)
	}
	if !strings.Contains(check.stdout, "dead reference: memory/missing.md") {
		t.Errorf("generated [pointers] should catch the dead reference:\n%s", check.stdout)
	}
	if !strings.Contains(check.stdout, ".DS_Store") {
		t.Errorf("generated [junk] should catch the planted .DS_Store:\n%s", check.stdout)
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if got := run(t, nil, "init", dir); got.code != 0 {
		t.Fatalf("first init exited %d\n%s", got.code, got.stderr)
	}
	before, err := os.ReadFile(filepath.Join(dir, ".memlint.toml"))
	if err != nil {
		t.Fatal(err)
	}

	got := run(t, nil, "init", dir)
	if got.code != 2 {
		t.Errorf("second init exited %d, want 2", got.code)
	}
	if !strings.Contains(got.stderr, "refuses to overwrite") {
		t.Errorf("stderr should state the refusal, got: %q", got.stderr)
	}
	after, err := os.ReadFile(filepath.Join(dir, ".memlint.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("refused init must leave the existing config untouched")
	}
}

// An empty repository yields no evidence. The generated config must still be
// valid, and check must disclose that nothing is verified.
func TestInitEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	got := run(t, nil, "init", dir)
	if got.code != 0 {
		t.Fatalf("init exited %d\n%s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "no rules enabled") {
		t.Errorf("init must disclose that nothing is enabled, got: %q", got.stdout)
	}
	check := run(t, nil, "check", dir)
	if check.code != 0 {
		t.Fatalf("check exited %d\n%s", check.code, check.stderr)
	}
	if !strings.Contains(check.stdout, "no rules enabled") {
		t.Errorf("check must disclose that nothing ran, got: %q", check.stdout)
	}
}

// The default path is the working directory.
func TestDefaultPathIsCurrentDirectory(t *testing.T) {
	abs, err := filepath.Abs(fixture("fixture-clean"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binPath, "check")
	cmd.Dir = abs
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exited with error: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "clean") {
		t.Errorf("got %q", out)
	}
}
