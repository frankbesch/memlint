package report

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocURL(t *testing.T) {
	tests := []struct{ code, want string }{
		{"pointers/dead-ref", DocBase + "#pointersdead-ref"},
		{"append_only/no-baseline", DocBase + "#append_onlyno-baseline"},
		{"mirrors/unverifiable", DocBase + "#unverifiable"},
		{"tokens/unverifiable", DocBase + "#unverifiable"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := DocURL(tt.code); got != tt.want {
			t.Errorf("DocURL(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// Every finding code the lint package can construct must have a heading in
// docs/findings.md — a new code cannot ship undocumented. Static codes are
// found by scanning the lint sources for "<rule>/<kind>" string literals; the
// dynamic <rule>/unverifiable family is covered by the shared "## unverifiable"
// heading that DocURL maps it to.
func TestEveryCodeIsDocumented(t *testing.T) {
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "findings.md"))
	if err != nil {
		t.Fatalf("reading docs/findings.md: %v", err)
	}
	headings := map[string]bool{}
	for _, line := range strings.Split(string(docs), "\n") {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			headings[strings.TrimSpace(h)] = true
		}
	}
	if !headings["unverifiable"] {
		t.Error("docs/findings.md lacks the shared \"## unverifiable\" heading")
	}

	rules := map[string]bool{
		"mirrors": true, "append_only": true, "blocks": true,
		"human_brief": true, "pointers": true, "junk": true, "tokens": true,
		"ids": true,
	}
	codeRe := regexp.MustCompile(`"([a-z_]+)/([a-z-]+)"`)

	entries, err := os.ReadDir(filepath.Join("..", "lint"))
	if err != nil {
		t.Fatalf("reading lint package dir: %v", err)
	}
	found := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join("..", "lint", name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, m := range codeRe.FindAllStringSubmatch(string(src), -1) {
			// The literal set also matches import paths ("io/fs") and example
			// paths in comments; only known rule prefixes are finding codes.
			if !rules[m[1]] {
				continue
			}
			code := m[1] + "/" + m[2]
			found++
			if !headings[code] {
				t.Errorf("code %q (in %s) has no \"## %s\" heading in docs/findings.md", code, name, code)
			}
		}
	}
	if found < 20 {
		t.Errorf("scan found only %d code literals — the source scan is likely broken", found)
	}
}
