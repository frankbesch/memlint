// Package lint evaluates memlint's rules against a repository root.
//
// Every rule is read-only. Nothing in this package creates, modifies, moves, or
// deletes a file, and there is no autofix path to add one to.
//
// Failure policy: a rule that cannot evaluate an invariant emits a RED
// "could not verify" finding rather than returning an error. A checker that
// silently skipped a check it was asked to perform would be worse than one that
// reports a failure, so unverifiable is treated as failed.
package lint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

// Run evaluates every enabled rule against root. root must already exist; the
// caller is responsible for that startup check.
func Run(root string, cfg *config.Config) Result {
	r := newRunner(root)

	if cfg.Mirrors != nil {
		checkMirrors(r, cfg.Mirrors)
	}
	if cfg.AppendOnly != nil {
		checkAppendOnly(r, cfg.AppendOnly)
	}
	if cfg.Blocks != nil {
		checkBlocks(r, cfg.Blocks)
	}
	if cfg.HumanBrief != nil {
		checkHumanBrief(r, cfg.HumanBrief)
	}
	if cfg.Pointers != nil {
		checkPointers(r, cfg.Pointers)
	}
	if cfg.Junk != nil {
		checkJunk(r, cfg.Junk)
	}
	if cfg.Tokens != nil {
		checkTokens(r, cfg.Tokens)
	}

	sortFindings(r.findings)
	return Result{
		Findings:     r.findings,
		RulesRun:     cfg.RuleCount(),
		FilesChecked: len(r.checked),
	}
}

type runner struct {
	// root is the cleaned absolute target root, used for joining.
	root string
	// rootEval is root with symlinks resolved, used for containment checks.
	// On macOS /tmp and /var are themselves symlinks, so comparing a resolved
	// candidate against an unresolved root would report false escapes.
	rootEval string

	findings []Finding
	checked  map[string]struct{}
}

func newRunner(root string) *runner {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = filepath.Clean(root)
	}
	evaled, err := filepath.EvalSymlinks(abs)
	if err != nil {
		evaled = abs
	}
	return &runner{root: abs, rootEval: evaled, checked: map[string]struct{}{}}
}

func (r *runner) add(f Finding) { r.findings = append(r.findings, f) }

// mark records that a path was actually inspected, for the clean-line count.
func (r *runner) mark(rel string) { r.checked[rel] = struct{}{} }

// red and yellow require a code so that no construction site can forget one:
// the finding vocabulary in SPEC.md is enforced by the compiler, not by review.
func (r *runner) red(rule, code, path, msg string) {
	r.add(Finding{Rule: rule, Code: code, Severity: SeverityRed, Path: path, Message: msg})
}

func (r *runner) yellow(rule, code, path, msg string) {
	r.add(Finding{Rule: rule, Code: code, Severity: SeverityYellow, Path: path, Message: msg})
}

// cannotVerify reports a check that could not be completed. This is RED by
// design: an unevaluated invariant must never be reported as clean.
func (r *runner) cannotVerify(rule, path string, err error) {
	r.red(rule, rule+"/unverifiable", path, fmt.Sprintf("could not verify: %v", unwrapPathErr(err)))
}

// unwrapPathErr strips the redundant absolute path an *os.PathError carries,
// since findings already name the path in their own field.
func unwrapPathErr(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// resolve maps a root-relative path to an absolute one, refusing anything that
// leaves the target root either lexically or through a symlink.
func (r *runner) resolve(rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", false
	}
	clean := config.CleanRel(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	abs := filepath.Join(r.root, filepath.FromSlash(clean))
	if !within(r.root, abs) {
		return "", false
	}
	// A symlink can point outside the root even when the path is lexically
	// inside it. Only checkable once the path exists; if it does not, the
	// lexical check above is already sufficient.
	if evaled, err := filepath.EvalSymlinks(abs); err == nil && !within(r.rootEval, evaled) {
		return "", false
	}
	return abs, true
}

// within reports whether p is base or lives under it.
func within(base, p string) bool {
	if p == base {
		return true
	}
	return strings.HasPrefix(p, base+string(os.PathSeparator))
}

// relSlash renders an absolute path relative to the root in display form.
func (r *runner) relSlash(abs string) string {
	rel, err := filepath.Rel(r.root, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// matchGlobs reports the first glob in globs matching name, checking the
// slash-normalized relative path and, when wantBase is set, the basename too.
// Returning only the first match is what keeps a file with three matching globs
// from producing three findings.
func matchGlobs(globs []string, rel string, wantBase bool) (string, bool) {
	base := path.Base(rel)
	for _, g := range globs {
		if ok, err := path.Match(g, rel); err == nil && ok {
			return g, true
		}
		if wantBase {
			if ok, err := path.Match(g, base); err == nil && ok {
				return g, true
			}
		}
	}
	return "", false
}

// walkFiles visits every entry under root, skipping .git. Symlinked
// directories are not followed: fs.WalkDir does not descend into them, which is
// the behavior we want, since a symlink out of the tree must not widen the
// scope of a check.
func (r *runner) walk(rule string, fn func(rel string, d fs.DirEntry) error) {
	err := filepath.WalkDir(r.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			r.cannotVerify(rule, r.relSlash(p), err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == r.root {
			return nil
		}
		rel := r.relSlash(p)
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		return fn(rel, d)
	})
	if err != nil {
		r.cannotVerify(rule, ".", err)
	}
}

// sortedKeys returns map keys in a deterministic order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
