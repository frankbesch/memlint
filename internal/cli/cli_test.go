package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if doc.Summary.Red != 7 || doc.Summary.Yellow != 2 {
		t.Errorf("got %d red / %d yellow, want 7 / 2", doc.Summary.Red, doc.Summary.Yellow)
	}
	if len(doc.Findings) != 9 {
		t.Errorf("got %d findings, want 9", len(doc.Findings))
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
