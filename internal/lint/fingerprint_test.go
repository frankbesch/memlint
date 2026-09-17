package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// copyTree copies a fixture into a fresh temp dir, so every mtime is new and
// the path differs — neither may move the fingerprint.
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

// G1: deterministic, and independent of mtime and absolute location.
func TestFingerprintIsContentBased(t *testing.T) {
	root := fixture(t, "fixture-clean")
	a, err := Fingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hex64.MatchString(a) {
		t.Fatalf("fingerprint %q is not 64 hex", a)
	}
	b, _ := Fingerprint(root)
	if a != b {
		t.Errorf("two runs differ: %s vs %s", a, b)
	}
	// The temp copy has no .git, so it takes the walk path; the fixture sits
	// inside the memvet repository, so it takes the git path. Both must
	// agree on the same set of files and bytes.
	c, err := Fingerprint(copyTree(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if a != c {
		t.Errorf("copy differs from original: %s vs %s", a, c)
	}
}

// G2: one byte moves it; restoring the byte restores it.
func TestFingerprintMovesWithContent(t *testing.T) {
	root := copyTree(t, fixture(t, "fixture-clean"))
	before, _ := Fingerprint(root)
	p := filepath.Join(root, "memory", "decisions.md")
	orig, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(append([]byte{}, orig...), 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, _ := Fingerprint(root)
	if moved == before {
		t.Error("appending a byte did not move the fingerprint")
	}
	if err := os.WriteFile(p, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	after, _ := Fingerprint(root)
	if after != before {
		t.Errorf("restoring the byte did not restore the fingerprint: %s vs %s", after, before)
	}
}

func TestFingerprintSkipsDotGit(t *testing.T) {
	root := copyTree(t, fixture(t, "fixture-clean"))
	before, _ := Fingerprint(root)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, _ := Fingerprint(root)
	if after != before {
		t.Error(".git contents moved the fingerprint")
	}
}

func TestExpectTree(t *testing.T) {
	res := Result{RulesRun: 1, Tree: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	res.ExpectTree("0123456789ab")
	if len(res.Findings) != 0 {
		t.Fatalf("matching prefix produced %d findings", len(res.Findings))
	}
	res.ExpectTree(res.Tree)
	if len(res.Findings) != 0 {
		t.Fatalf("matching full fingerprint produced %d findings", len(res.Findings))
	}
	res.ExpectTree("ffffffffffff")
	if len(res.Findings) != 1 || res.Findings[0].Code != "tree/moved" || res.Findings[0].Severity != SeverityRed {
		t.Fatalf("got %+v", res.Findings)
	}
	if !TreeMatches(res.Tree, "0123456789AB") {
		t.Error("prefix match must be case-insensitive")
	}
	if TreeMatches(res.Tree, "0123456789a") {
		t.Error("an 11-hex prefix must not match")
	}
}
