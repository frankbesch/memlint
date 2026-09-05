package lint

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

func runSecrets(root string, cfg *config.Secrets) Result {
	return Run(root, &config.Config{Secrets: cfg})
}

func TestSecretsBuiltinPatterns(t *testing.T) {
	root := writeTree(t, map[string]string{
		"notes/card.md":  "gift card 4111 1111 1111 1111 pin 1234\n",
		"notes/aws.md":   "key AKIAIOSFODNN7EXAMPLE\n",
		"notes/gh.md":    "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij\n",
		"notes/ant.md":   "sk-ant-api03-" + strings.Repeat("a", 40) + "\n",
		"notes/pem.md":   "-----BEGIN RSA PRIVATE KEY-----\n",
		"notes/clean.md": "order 1234-5678, D-102, phone 512-555-0100, not a card 1234567890123\n",
	})
	res := runSecrets(root, &config.Secrets{Globs: []string{"notes/*.md"}})
	wantCounts(t, res, 5, 0)
	for _, f := range res.Findings {
		if f.Path == "notes/clean.md" {
			t.Errorf("false positive: %s", f.Message)
		}
		if strings.Contains(f.Message, "4111 1111 1111 1111") || strings.Contains(f.Message, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("a secret must never be echoed back: %s", f.Message)
		}
		if f.Code != "secrets/match" {
			t.Errorf("code %s", f.Code)
		}
	}
	if !hasFinding(res, "secrets", "notes/card.md", "card number") {
		t.Errorf("Luhn-valid card must be found:\n%s", dump(res))
	}
}

// Extra patterns are the repo's own; a stale zero-match glob is YELLOW.
func TestSecretsCustomPatternAndNoMatch(t *testing.T) {
	root := writeTree(t, map[string]string{"a.md": "internal id FB-SECRET-9\n"})
	res := runSecrets(root, &config.Secrets{Globs: []string{"*.md", "gone/*.md"}, Patterns: []string{`FB-SECRET-\d+`}})
	wantCounts(t, res, 1, 1)
	wantMessage(t, res, "custom pattern")
}
