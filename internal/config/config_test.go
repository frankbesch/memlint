package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withConfig writes a .memlint.toml into a fresh directory and loads it.
func withConfig(t *testing.T, body string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(dir)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("a missing config must be an error, not an empty config")
	}
	if !strings.Contains(err.Error(), FileName) {
		t.Errorf("error should name the file it looked for: %v", err)
	}
}

// An empty config is valid and disables everything. It is not an error: the
// caller reports that no rules ran, which keeps "clean" from implying
// "verified" without inventing a failure.
func TestLoadEmptyConfigIsValid(t *testing.T) {
	cfg, err := withConfig(t, "# nothing enabled\n")
	if err != nil {
		t.Fatalf("empty config should load: %v", err)
	}
	if got := cfg.RuleCount(); got != 0 {
		t.Errorf("got %d rules, want 0", got)
	}
}

func TestSectionPresenceEnablesRules(t *testing.T) {
	cfg, err := withConfig(t, `
[junk]
globs = ["*.tmp"]

[tokens]
watch = ["memory/*.md"]
budget = 100
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mirrors != nil || cfg.AppendOnly != nil || cfg.Blocks != nil ||
		cfg.HumanBrief != nil || cfg.Pointers != nil {
		t.Error("absent sections must stay nil")
	}
	if cfg.Junk == nil || cfg.Tokens == nil {
		t.Error("present sections must be non-nil")
	}
	if got := cfg.RuleCount(); got != 2 {
		t.Errorf("got %d rules, want 2", got)
	}
}

// Every case here must be rejected before a single rule runs. Silently ignoring
// a key memlint does not understand would let a repository look verified when
// the check its author wrote was never executed.
func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"malformed toml", "[junk\nglobs = []", "expected"},
		{"unknown section", "[mirror]\npairs = []", "unknown key"},
		{"unknown field", "[junk]\nglobs = [\"*.tmp\"]\nrecursive = true", "unknown key"},

		{"empty mirrors", "[mirrors]\n", "present but empty"},
		{"empty append_only", "[append_only]\n", "present but empty"},
		{"empty junk", "[junk]\n", "present but empty"},
		{"empty tokens", "[tokens]\nbudget = 10", "present but empty"},
		{"pointers without roots", "[pointers]\nfiles = [\"a.md\"]", "present but empty"},

		{"mirror pair too short", `[mirrors]
pairs = [["a.md"]]`, "exactly 2 paths"},
		{"mirror pair too long", `[mirrors]
pairs = [["a.md", "b.md", "c.md"]]`, "exactly 2 paths"},
		{"mirror pair with identical sides", `[mirrors]
pairs = [["a.md", "./a.md"]]`, "same path"},
		{"duplicate mirror pair regardless of order", `[mirrors]
pairs = [["a.md", "b.md"], ["b.md", "a.md"]]`, "duplicate pair"},

		{"zero budget", `[tokens]
watch = ["a.md"]
budget = 0`, "positive"},
		{"negative budget", `[tokens]
watch = ["a.md"]
budget = -1`, "positive"},
		{"limit at budget", `[tokens]
watch = ["a.md"]
budget = 10
limit = 10`, "greater than budget"},
		{"limit below budget", `[tokens]
watch = ["a.md"]
budget = 10
limit = 5`, "greater than budget"},

		{"invalid glob", `[junk]
globs = ["[unclosed"]`, "invalid glob"},
		{"** inside a segment", `[junk]
globs = ["a**b/*.tmp"]`, "whole path segment"},
		{"duplicate glob", `[junk]
globs = ["*.tmp", "*.tmp"]`, "duplicate glob"},

		{"absolute path", `[append_only]
files = ["/etc/passwd"]`, "must be relative"},
		{"parent traversal", `[append_only]
files = ["../outside.md"]`, "must not escape"},
		{"duplicate file", `[append_only]
files = ["a.md", "./a.md"]`, "duplicate path"},
		{"negative header_lines", `[append_only]
files = ["a.md"]
header_lines = -1`, "header_lines"},

		{"empty blocks", "[blocks]\n", "present but empty"},
		{"blocks without markers", `[blocks]
files = ["AGENTS.md"]`, "present but empty"},
		{"blocks with identical markers", `[blocks]
files = ["AGENTS.md"]
start = "<!-- X -->"
end = "<!-- X -->"`, "must differ"},
		{"blocks marker contains the other", `[blocks]
files = ["AGENTS.md"]
start = "<!-- AGENT -->"
end = "<!-- AGENT --> END"`, "must not contain each other"},
		{"blocks multiline marker", `[blocks]
files = ["AGENTS.md"]
start = "<!-- A\nB -->"
end = "<!-- C -->"`, "single line"},

		{"empty human_brief", "[human_brief]\n", "present but empty"},
		{"append_only headers key not listed", `[append_only]
files = ["a.md"]
headers = { "b.md" = 3 }`, "headers"},
		{"append_only headers negative", `[append_only]
files = ["a.md"]
headers = { "a.md" = -1 }`, "headers"},

		{"empty ids", "[ids]\n", "present but empty"},
		{"ids bad pattern", `[ids]
files = ["a.md"]
pattern = "^(D-\\d{3}"`, "pattern"},
		{"ids duplicate file", `[ids]
files = ["a.md", "./a.md"]`, "duplicate entry"},
		{"ids empty known entry", `[ids]
files = ["a.md"]
known = ["D-102", ""]`, "known[1]"},
		{"ids duplicate known entry", `[ids]
files = ["a.md"]
known = ["D-102", "D-102"]`, "known[1]"},
		{"human_brief without authors", `[human_brief]
files = ["INSTRUCTIONS.md"]`, "present but empty"},
		{"human_brief empty author", `[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = [" "]`, "must not be empty"},
		{"human_brief duplicate author ignoring case", `[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = ["bot", "Bot"]`, "duplicate identity"},

		{"empty root", `[pointers]
files = ["a.md"]
roots = [""]`, "must not be empty"},
		{"absolute root", `[pointers]
files = ["a.md"]
roots = ["/memory"]`, "must not be absolute"},
		{"multi-segment root", `[pointers]
files = ["a.md"]
roots = ["memory/notes"]`, "single path segment"},
		{"duplicate root", `[pointers]
files = ["a.md"]
roots = ["memory", "memory"]`, "duplicate root"},
		{"invalid glob in pointer files", `[pointers]
files = ["[unclosed"]
roots = ["memory"]`, "invalid glob"},
		{"duplicate glob in pointer files", `[pointers]
files = ["memory/*.md", "memory/*.md"]
roots = ["memory"]`, "duplicate entry"},
		{"absolute literal in pointer files", `[pointers]
files = ["/memory/index.md"]
roots = ["memory"]`, "must be relative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := withConfig(t, tt.body)
			if err == nil {
				t.Fatalf("expected rejection, got a valid config")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestLoadAcceptsAFullConfig(t *testing.T) {
	cfg, err := withConfig(t, `
[mirrors]
pairs = [["CLAUDE.md", "docs/CLAUDE.md"], ["sync/left", "sync/right"]]

[append_only]
files = ["memory/decisions.md"]

[blocks]
files = ["AGENTS.md"]
start = "<!-- AGENT:START -->"
end = "<!-- AGENT:END -->"

[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = ["openwiki[bot]"]

[pointers]
files = ["memory/index.md"]
roots = ["memory", "docs"]

[junk]
globs = ["*.tmp", ".DS_Store"]

[tokens]
watch = ["memory/*.md"]
budget = 2000
`)
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if got := cfg.RuleCount(); got != 7 {
		t.Errorf("got %d rules, want 7", got)
	}
}

func TestCleanRel(t *testing.T) {
	cases := map[string]string{
		"a.md":        "a.md",
		"./a.md":      "a.md",
		"a//b.md":     "a/b.md",
		"a/./b.md":    "a/b.md",
		"a/c/../b.md": "a/b.md",
		"a/":          "a",
	}
	for in, want := range cases {
		if got := CleanRel(in); got != want {
			t.Errorf("CleanRel(%q) = %q, want %q", in, got, want)
		}
	}
}

// header_lines is optional: absent means zero, and the struct a pre-v0.7
// config loads to is unchanged.
func TestAppendOnlyHeaderLines(t *testing.T) {
	cfg, err := withConfig(t, "[append_only]\nfiles = [\"a.md\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppendOnly.HeaderLines != 0 {
		t.Errorf("absent header_lines must be 0, got %d", cfg.AppendOnly.HeaderLines)
	}
	cfg, err = withConfig(t, "[append_only]\nfiles = [\"a.md\"]\nheader_lines = 4\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppendOnly.HeaderLines != 4 {
		t.Errorf("got header_lines %d, want 4", cfg.AppendOnly.HeaderLines)
	}
}

// [ids] pattern defaults to the D-### form and can be replaced.
func TestIDsPatternDefault(t *testing.T) {
	cfg, err := withConfig(t, "[ids]\nfiles = [\"a.md\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IDs == nil || cfg.IDs.Pattern != "" || cfg.IDs.EffectivePattern() != DefaultIDPattern {
		t.Errorf("absent pattern must resolve to the default, got %+v", cfg.IDs)
	}
	if cfg.RuleCount() != 1 {
		t.Errorf("[ids] must count as a rule, got %d", cfg.RuleCount())
	}
	cfg, err = withConfig(t, "[ids]\nfiles = [\"a.md\", \"b/*.md\"]\npattern = \"^(L-\\\\d+)\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IDs.EffectivePattern() != `^(L-\d+)` {
		t.Errorf("got pattern %q", cfg.IDs.EffectivePattern())
	}
}

// Recursive ** is a whole-segment wildcard since v0.8.
func TestRecursiveGlobsAccepted(t *testing.T) {
	cfg, err := withConfig(t, "[junk]\nglobs = [\"**/*.tmp\", \"memory/**\", \"a/**/b/*.md\"]\n")
	if err != nil {
		t.Fatalf("** globs must load: %v", err)
	}
	if len(cfg.Junk.Globs) != 3 {
		t.Errorf("got %d globs", len(cfg.Junk.Globs))
	}
}
