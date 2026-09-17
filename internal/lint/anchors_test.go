package lint

import (
	"testing"

	"github.com/frankbesch/memvet/internal/config"
)

func TestHeadingSlug(t *testing.T) {
	cases := map[string]string{
		"Plan":                           "plan",
		"Open Loops & Next Actions":      "open-loops--next-actions",
		"D-001 | 2026-07-01 | adopt":     "d-001--2026-07-01--adopt",
		"`code` in *emphasis* [link](x)": "code-in-emphasis-link",
		"  Trailing hashes ##":           "trailing-hashes",
		"Ünïcode Wörds":                  "ünïcode-wörds",
		"snake_case_ok":                  "snake_case_ok",
	}
	for in, want := range cases {
		if got := headingSlug(in); got != want {
			t.Errorf("headingSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownAnchors(t *testing.T) {
	src := "# Title\n\n## Notes\n\n```\n## not a heading\n```\n\n## Notes\n\n### Notes {#custom}\n\n<a id=\"explicit\"></a>\n<a name=\"named\">x</a>\n\n~~~md\n# fenced too\n~~~\n"
	got := markdownAnchors(src)
	for _, want := range []string{"title", "notes", "notes-1", "notes-2", "custom", "explicit", "named"} {
		if !got[want] {
			t.Errorf("missing anchor %q in %v", want, sortedKeys(got))
		}
	}
	for _, absent := range []string{"not-a-heading", "fenced-too"} {
		if got[absent] {
			t.Errorf("fenced heading %q must not be an anchor", absent)
		}
	}
}

func TestDeadAnchorRule(t *testing.T) {
	cfg := &config.Config{Pointers: &config.Pointers{Files: []string{"memory/index.md"}, Roots: []string{"memory"}}}

	t.Run("missing heading is red with a hint", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "see `memory/live.md#nowhere` and `memory/live.md#plan`\n",
			"memory/live.md":  "# Live\n\n## Plan\n",
		})
		res := Run(root, cfg)
		wantCounts(t, res, 1, 0)
		f := res.Findings[0]
		if f.Code != "pointers/dead-anchor" || f.RelatedPath != "memory/live.md" || f.Line != 1 {
			t.Fatalf("got %+v", f)
		}
		wantMessage(t, res, `dead anchor: memory/live.md has no heading or anchor "nowhere"`)
		if f.Detail != "anchors present: #live #plan" {
			t.Errorf("detail %q", f.Detail)
		}
	})

	t.Run("two anchors on one file are two claims", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "`memory/live.md#a`\n`memory/live.md#b`\n`memory/live.md#a`\n",
			"memory/live.md":  "# Live\n",
		})
		res := Run(root, cfg)
		wantCounts(t, res, 2, 0)
	})

	t.Run("dead base stays dead-ref only", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "`memory/gone.md#plan`\n",
		})
		res := Run(root, cfg)
		wantCounts(t, res, 1, 0)
		if res.Findings[0].Code != "pointers/dead-ref" {
			t.Errorf("got %s", res.Findings[0].Code)
		}
	})

	t.Run("non-markdown target is never anchor-checked", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md":  "`memory/page.html#top`\n",
			"memory/page.html": "<p>no anchors</p>\n",
		})
		res := Run(root, cfg)
		wantCounts(t, res, 0, 0)
	})

	t.Run("explicit html anchor and custom id resolve", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"memory/index.md": "`memory/live.md#explicit` `memory/live.md#custom`\n",
			"memory/live.md":  "<a id=\"explicit\"></a>\n\n## Heading {#custom}\n",
		})
		res := Run(root, cfg)
		wantCounts(t, res, 0, 0)
	})
}
