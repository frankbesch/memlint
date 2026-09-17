package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint is the SHA-256 of the tree memvet would check under root: for
// every visible regular file, sorted by root-relative slash path, the line
// "path\0size\0sha256(content)\n". It is content-based on purpose — a clone,
// a checkout, or a touch does not move it — so it serves as a receipt: the
// verdict names the tree it judged, and a later step can demand that same
// tree (--expect-tree).
//
// Visible means what a commit could contain when root is inside a git
// repository (tracked plus untracked-not-ignored, minus paths that no longer
// exist); without git, every regular file under root except .git/. Symlinks
// are never followed. Read-only.
func Fingerprint(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rels, err := gitVisible(abs)
	if err != nil {
		rels, err = walkVisible(abs)
		if err != nil {
			return "", err
		}
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		p := filepath.Join(abs, filepath.FromSlash(rel))
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() {
			// Deleted-but-tracked files and symlinks are not content.
			continue
		}
		sum, err := fileSHA(p)
		if err != nil {
			return "", fmt.Errorf("fingerprint: %s: %v", rel, unwrapPathErr(err))
		}
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", rel, info.Size(), sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// TreeMatches reports whether got starts with want. want may be the full
// 64-hex fingerprint or a prefix of at least 12 hex characters.
func TreeMatches(got, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	return len(want) >= 12 && strings.HasPrefix(got, want)
}

// ShortTree is the 12-hex form shown in text summaries.
func ShortTree(fp string) string {
	if len(fp) > 12 {
		return fp[:12]
	}
	return fp
}

func gitVisible(abs string) ([]string, error) {
	out, err := gitOut(abs, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var rels []string
	for _, p := range strings.Split(out, "\x00") {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		rels = append(rels, filepath.ToSlash(p))
	}
	return rels, nil
}

func walkVisible(abs string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != abs && d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	return rels, err
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
