package lint

import (
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/config"
)

// TestExtractRefs covers what counts as a repo-path-like reference at all,
// before any root filtering. The skip rules are decided here, which is why the
// spec's URL, placeholder, and anchor cases are asserted at this level: the
// roots filter in CheckableRefs could mask them by accident.
func TestExtractRefs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		// --- forms that must be extracted ---
		{"backticked path", "see `memory/foo.md` for context", []string{"memory/foo.md"}},
		{"bare path", "see memory/foo.md for context", []string{"memory/foo.md"}},
		{"markdown link", "see [the note](memory/foo.md)", []string{"memory/foo.md"}},
		{"image link", "![diagram](docs/diagram.png)", []string{"docs/diagram.png"}},
		{"wrapped in parens", "the note (memory/foo.md) is stale", []string{"memory/foo.md"}},
		{"trailing sentence punctuation", "it lives in memory/foo.md.", []string{"memory/foo.md"}},
		{"trailing comma", "memory/foo.md, memory/bar.md", []string{"memory/foo.md", "memory/bar.md"}},
		{"leading ./ is normalized away", "see `./memory/foo.md`", []string{"memory/foo.md"}},
		{"directory reference", "everything under `memory/notes/`", []string{"memory/notes"}},

		// --- spec-mandated skips ---
		{"SPEC: url is skipped", "see https://example.com/notes", nil},
		{"SPEC: placeholder is skipped", "`reviews/YYYY-MM-DD-<topic>.html`", nil},

		// --- anchors (checkable since v0.6) ---
		{"anchored ref yields its base path", "`memory/gone.md#section`", []string{"memory/gone.md"}},
		{"empty anchor still yields the base", "`memory/gone.md#`", []string{"memory/gone.md"}},
		{"anchored and bare dedup to one target", "`memory/gone.md#a` and `memory/gone.md`", []string{"memory/gone.md"}},
		{"more than one # is not a path+anchor", "`memory/gone.md#a#b`", nil},
		{"bare fragment has no slash", "see #section for details", nil},
		{"url with fragment is still a url", "https://example.com/notes#section", nil},
		{"anchor after a placeholder base is still skipped", "`memory/<name>.md#x`", nil},

		// --- other skips ---
		{"no slash is not a path", "see CLAUDE.md for context", nil},
		{"angle-bracket placeholder", "`memory/<name>.md`", nil},
		{"glob is a placeholder for a set", "watch `memory/*.md`", nil},
		{"absolute path is out of scope", "`/etc/passwd`", nil},
		{"parent escape is refused", "`../outside/x.md`", nil},
		{"spaces inside backticks", "`memory/two words.md`", nil},

		// --- ordering and dedup ---
		{
			"deduplicated, first occurrence wins",
			"`memory/foo.md` and again memory/foo.md",
			[]string{"memory/foo.md"},
		},
		{
			"ordered by position within a line",
			"docs/b.md then memory/a.md",
			[]string{"docs/b.md", "memory/a.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, ref := range ExtractRefs(tt.src) {
				got = append(got, ref.Target)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("ExtractRefs(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

// TestCheckableRefs covers the roots gate: which extracted references memlint
// is actually responsible for verifying. Each case sets roots so that the
// reason for the outcome is unambiguous -- a reference must be skipped because
// of the rule under test, not because its root happened to be unlisted.
func TestCheckableRefs(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		roots []string
		want  []string
	}{
		{
			// SPEC: "reading/daily" with roots not containing "reading" -> skip.
			name:  "SPEC: unlisted first segment is skipped",
			src:   "the log lives in `reading/daily`",
			roots: []string{"memory", "docs"},
			want:  nil,
		},
		{
			// SPEC: "memory/x.md" -> check.
			name:  "SPEC: listed first segment is checked",
			src:   "the note lives in `memory/x.md`",
			roots: []string{"memory", "docs"},
			want:  []string{"memory/x.md"},
		},
		{
			// SPEC: "reviews/YYYY-MM-DD-<topic>.html" -> skip. Roots include
			// "reviews" so this can only be skipped by the placeholder rule.
			name:  "SPEC: placeholder skipped even when its root is listed",
			src:   "published at `reviews/YYYY-MM-DD-<topic>.html`",
			roots: []string{"reviews"},
			want:  nil,
		},
		{
			// SPEC: "https://a/b" -> skip.
			name:  "SPEC: url is never checkable",
			src:   "documented at https://a/b",
			roots: []string{"https:", "a"},
			want:  nil,
		},
		{
			// Roots gate the BASE path: the anchor plays no part in the filter.
			name:  "anchored ref is checkable by its base's root",
			src:   "see `memory/gone.md#section`",
			roots: []string{"memory"},
			want:  []string{"memory/gone.md"},
		},
		{
			name:  "a listed root with an unlisted one alongside it",
			src:   "`memory/keep.md` and `scratch/drop.md`",
			roots: []string{"memory"},
			want:  []string{"memory/keep.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, ref := range CheckableRefs(tt.src, tt.roots) {
				got = append(got, ref.Target)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("CheckableRefs(%q, %v) = %v, want %v", tt.src, tt.roots, got, tt.want)
			}
		})
	}
}

func TestExtractRefsReportsFirstOccurrenceLine(t *testing.T) {
	src := "# Title\n\nnothing here\nsee `memory/foo.md`\nand again memory/foo.md\n"
	refs := ExtractRefs(src)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(refs), refs)
	}
	if refs[0].Line != 4 {
		t.Errorf("got line %d, want 4 (the first occurrence)", refs[0].Line)
	}
}

// A single line longer than bufio.Scanner's default 64 KiB buffer must not
// cause references to be silently dropped. A reference lost by the reader is
// indistinguishable from one that resolves, which would be a silent false pass.
func TestExtractRefsHandlesVeryLongLine(t *testing.T) {
	long := strings.Repeat("x", 70_000)
	src := "intro\n" + long + " `memory/foo.md`\n"

	refs := ExtractRefs(src)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(refs), refs)
	}
	if refs[0].Target != "memory/foo.md" || refs[0].Line != 2 {
		t.Errorf("got %+v, want memory/foo.md on line 2", refs[0])
	}
}

func TestPointersRule(t *testing.T) {
	t.Run("dead reference is red, live reference is silent", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "- `memory/live.md`\n- `memory/dead.md`\n",
			"memory/live.md":  "here\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "dead reference: memory/dead.md")
		if got := res.Findings[0].Line; got != 2 {
			t.Errorf("got line %d, want 2", got)
		}
	})

	t.Run("a directory target counts as existing", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md":   "see `memory/notes/`\n",
			"memory/notes/a.md": "a\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 0, 0)
	})

	t.Run("missing source file is red", func(t *testing.T) {
		root := writeTree(t, map[string]string{"other.md": "x\n"})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "pointer source file does not exist")
	})

	t.Run("one finding per target, not per occurrence", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "`memory/dead.md`\n`memory/dead.md`\nmemory/dead.md\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 1, 0)
	})

	t.Run("anchored dead base is red and names the anchored form", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "see `memory/gone.md#plan`\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 1, 0)
		wantMessage(t, res, "dead reference: memory/gone.md does not exist (referenced as memory/gone.md#plan)")
	})

	t.Run("anchored ref to a live base is silent", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "see `memory/live.md#plan`\n",
			"memory/live.md":  "here\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 0, 0)
	})

	t.Run("files glob expands to every matching source", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/a.md": "`memory/dead-one.md`\n",
			"memory/b.md": "`memory/dead-two.md`\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/*.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 2, 0)
		wantMessage(t, res, "dead reference: memory/dead-one.md")
		wantMessage(t, res, "dead reference: memory/dead-two.md")
	})

	t.Run("glob matches the relative path, never the basename", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"nested/index.md": "`memory/dead.md`\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"*.md"}, Roots: []string{"memory"},
		}})
		// "*.md" names root-level markdown only: the nested file must not
		// satisfy it by basename, so the glob matches nothing (yellow) and the
		// nested file's dead reference is never scanned.
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "files glob matched no files")
	})

	t.Run("files glob matching nothing is yellow", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "clean\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md", "notes/*.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 0, 1)
		wantMessage(t, res, "files glob matched no files")
	})

	t.Run("glob and literal covering the same file scan it once", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "`memory/dead.md`\n",
		})
		res := Run(root, &config.Config{Pointers: &config.Pointers{
			Files: []string{"memory/index.md", "memory/*.md"}, Roots: []string{"memory"},
		}})
		wantCounts(t, res, 1, 0)
	})
}
