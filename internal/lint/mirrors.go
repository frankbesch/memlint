package lint

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleMirrors = "mirrors"

// checkMirrors verifies that each configured pair stays byte-identical.
func checkMirrors(r *runner, cfg *config.Mirrors) {
	for _, pair := range cfg.Pairs {
		checkMirrorPair(r, config.CleanRel(pair[0]), config.CleanRel(pair[1]))
	}
}

func checkMirrorPair(r *runner, a, b string) {
	aAbs, aInfo, ok := r.mirrorEndpoint(a, b)
	if !ok {
		return
	}
	bAbs, bInfo, ok := r.mirrorEndpoint(b, a)
	if !ok {
		return
	}

	switch {
	case aInfo.IsDir() && bInfo.IsDir():
		compareMirrorDirs(r, a, aAbs, b, bAbs)
	case aInfo.Mode().IsRegular() && bInfo.Mode().IsRegular():
		r.mark(a)
		r.mark(b)
		compareMirrorFiles(r, a, aAbs, b, bAbs)
	default:
		r.add(Finding{
			Rule: ruleMirrors, Code: "mirrors/kind-mismatch", Severity: SeverityRed, Path: a, RelatedPath: b,
			Message: fmt.Sprintf("mirror endpoints are not the same kind: %s is %s, %s is %s",
				a, kindOf(aInfo), b, kindOf(bInfo)),
		})
	}
}

// mirrorEndpoint resolves and stats one side of a pair, reporting RED if the
// endpoint is unusable. related is only used to make the message legible.
func (r *runner) mirrorEndpoint(p, related string) (string, os.FileInfo, bool) {
	abs, ok := r.resolve(p)
	if !ok {
		r.add(Finding{
			Rule: ruleMirrors, Code: "mirrors/escape", Severity: SeverityRed, Path: p, RelatedPath: related,
			Message: "mirror endpoint escapes the repository root",
		})
		return "", nil, false
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			r.add(Finding{
				Rule: ruleMirrors, Code: "mirrors/missing", Severity: SeverityRed, Path: p, RelatedPath: related,
				Message: "mirror endpoint does not exist",
			})
		} else {
			r.cannotVerify(ruleMirrors, p, err)
		}
		return "", nil, false
	}
	return abs, info, true
}

func kindOf(info os.FileInfo) string {
	switch {
	case info.IsDir():
		return "a directory"
	case info.Mode().IsRegular():
		return "a file"
	default:
		return "neither a file nor a directory"
	}
}

func compareMirrorFiles(r *runner, aRel, aAbs, bRel, bAbs string) {
	aData, err := os.ReadFile(aAbs)
	if err != nil {
		r.cannotVerify(ruleMirrors, aRel, err)
		return
	}
	bData, err := os.ReadFile(bAbs)
	if err != nil {
		r.cannotVerify(ruleMirrors, bRel, err)
		return
	}
	if bytes.Equal(aData, bData) {
		return
	}

	off := firstDiffOffset(aData, bData)
	msg := fmt.Sprintf("mirrored files differ at byte %d", off)
	// Line and column are only meaningful when the offset lands inside a file;
	// when one side is a strict prefix of the other it lands at the shorter
	// file's EOF, and the longer side still gives a usable position.
	src := aData
	if off >= len(aData) {
		src = bData
	}
	if off < len(src) {
		line, col := lineCol(src, off)
		msg = fmt.Sprintf("mirrored files differ at byte %d (line %d, col %d)", off, line, col)
	}
	r.add(Finding{
		Rule: ruleMirrors, Code: "mirrors/differ", Severity: SeverityRed, Path: aRel, RelatedPath: bRel,
		Message: msg,
		Detail:  fmt.Sprintf("%s is %d bytes, %s is %d bytes", aRel, len(aData), bRel, len(bData)),
	})
}

// firstDiffOffset returns the index of the first differing byte, or the length
// of the shorter input when one is a prefix of the other.
func firstDiffOffset(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// lineCol converts a byte offset to a 1-based line and column.
func lineCol(data []byte, off int) (int, int) {
	line, col := 1, 1
	for i := 0; i < off && i < len(data); i++ {
		if data[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

func compareMirrorDirs(r *runner, aRel, aAbs, bRel, bAbs string) {
	aFiles, err := collectMarkdown(aAbs)
	if err != nil {
		r.cannotVerify(ruleMirrors, aRel, err)
		return
	}
	bFiles, err := collectMarkdown(bAbs)
	if err != nil {
		r.cannotVerify(ruleMirrors, bRel, err)
		return
	}

	union := map[string]struct{}{}
	for _, f := range aFiles {
		union[f] = struct{}{}
	}
	for _, f := range bFiles {
		union[f] = struct{}{}
	}
	inA := setOf(aFiles)
	inB := setOf(bFiles)

	for _, rel := range sortedKeys(union) {
		aMember, bMember := path.Join(aRel, rel), path.Join(bRel, rel)
		_, hasA := inA[rel]
		_, hasB := inB[rel]
		switch {
		case hasA && !hasB:
			r.mark(aMember)
			r.add(Finding{
				Rule: ruleMirrors, Code: "mirrors/one-sided", Severity: SeverityRed, Path: aMember, RelatedPath: bMember,
				Message: fmt.Sprintf("present in %s but missing from %s", aRel, bRel),
			})
		case !hasA && hasB:
			r.mark(bMember)
			r.add(Finding{
				Rule: ruleMirrors, Code: "mirrors/one-sided", Severity: SeverityRed, Path: bMember, RelatedPath: aMember,
				Message: fmt.Sprintf("present in %s but missing from %s", bRel, aRel),
			})
		default:
			r.mark(aMember)
			r.mark(bMember)
			compareMirrorFiles(r,
				aMember, filepath.Join(aAbs, filepath.FromSlash(rel)),
				bMember, filepath.Join(bAbs, filepath.FromSlash(rel)))
		}
	}
}

// collectMarkdown lists every regular *.md file under dir, as slash-separated
// paths relative to dir. .git is skipped and symlinked directories are not
// followed.
func collectMarkdown(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != dir {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !hasMarkdownExt(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

func hasMarkdownExt(name string) bool {
	ext := filepath.Ext(name)
	return len(ext) == 3 && (ext[1] == 'm' || ext[1] == 'M') && (ext[2] == 'd' || ext[2] == 'D') && ext[0] == '.'
}

func setOf(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, i := range items {
		m[i] = struct{}{}
	}
	return m
}
