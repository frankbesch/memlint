package lint

import (
	"fmt"
	"os"
	"strings"

	"github.com/frankbesch/memlint/internal/config"
)

const ruleBlocks = "blocks"

// checkBlocks verifies that each configured file contains exactly one
// well-formed ownership block: one start marker, then one end marker.
//
// This is the structural half of the ownership-block convention. Whether the
// agent actually stayed inside its block is an authorship question the working
// tree cannot answer; what it can answer is whether the block an agent will
// rewrite on its next run is still unambiguous. A half-deleted marker means the
// next run rewrites the wrong span, which is exactly the silent drift memlint
// exists to catch.
//
// With mirror = true the content between the markers must also be identical
// across the listed files — the same agent-owned block embedded in several
// surfaces. The first listed file is the reference; a file whose block is
// structurally broken gets its structural finding only, since there is no
// unambiguous span to compare yet.
func checkBlocks(r *runner, cfg *config.Blocks) {
	type block struct {
		rel     string
		content string // between the markers, exclusive
		line    int    // 1-based line of the first content line
	}
	var ref *block
	for _, f := range cfg.Files {
		rel := config.CleanRel(f)
		content, span, ok := checkBlocksFile(r, rel, cfg.Start, cfg.End)
		if !ok || !cfg.Mirror {
			continue
		}
		b := &block{rel: rel, content: content[span[0]:span[1]], line: lineAt(content, span[0])}
		if ref == nil {
			ref = b
			continue
		}
		if b.content == ref.content {
			continue
		}
		line, was, now := firstDivergentLine([]byte(ref.content), []byte(b.content))
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/content-differ", Severity: SeverityRed,
			Path: b.rel, RelatedPath: ref.rel, Line: b.line + line - 1,
			Message: fmt.Sprintf("ownership block content differs from %s", ref.rel),
			Detail:  fmt.Sprintf("%s line %d: %s\n%s line %d: %s", ref.rel, ref.line+line-1, was, b.rel, b.line+line-1, now),
		})
	}
}

// checkBlocksFile reports the file's structural state. On a well-formed block
// it returns the content and the [start, end) byte span between the markers,
// with ok true; anything else has already been reported.
func checkBlocksFile(r *runner, rel, start, end string) (content string, span [2]int, ok bool) {
	abs, resolved := r.resolve(rel)
	if !resolved {
		r.red(ruleBlocks, "blocks/escape", rel, "path escapes the repository root")
		return
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			// The config declares this file carries an ownership block. A file
			// that does not exist cannot honor that contract.
			r.red(ruleBlocks, "blocks/missing", rel, "file with a declared ownership block does not exist")
			return
		}
		r.cannotVerify(ruleBlocks, rel, err)
		return
	}
	r.mark(rel)

	content = string(data)
	starts := markerOffsets(content, start)
	ends := markerOffsets(content, end)

	// One finding per file: the first structural problem is the one a repair
	// has to address before the later ones are even meaningful. Offsets rather
	// than line numbers decide ordering, so a block opened and closed on the
	// same line is still judged correctly.
	switch {
	case len(starts) == 0 && len(ends) == 0:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/no-markers", Severity: SeverityRed, Path: rel,
			Message: "ownership block missing: no markers found",
			Detail:  fmt.Sprintf("expected %q ... %q", start, end),
		})
	case len(starts) == 0:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/end-without-start", Severity: SeverityRed, Path: rel,
			Line:    lineAt(content, ends[0]),
			Message: "ownership block malformed: end marker without a start marker",
			Detail:  fmt.Sprintf("expected %q before it", start),
		})
	case len(ends) == 0:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/unterminated", Severity: SeverityRed, Path: rel,
			Line:    lineAt(content, starts[0]),
			Message: "ownership block unterminated: start marker has no end marker",
			Detail:  fmt.Sprintf("expected %q after it", end),
		})
	case len(starts) > 1:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/duplicate-start", Severity: SeverityRed, Path: rel,
			Line:    lineAt(content, starts[1]),
			Message: "ownership block malformed: duplicate start marker",
			Detail:  fmt.Sprintf("first start marker is on line %d", lineAt(content, starts[0])),
		})
	case len(ends) > 1:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/duplicate-end", Severity: SeverityRed, Path: rel,
			Line:    lineAt(content, ends[1]),
			Message: "ownership block malformed: duplicate end marker",
			Detail:  fmt.Sprintf("first end marker is on line %d", lineAt(content, ends[0])),
		})
	case ends[0] < starts[0]:
		r.add(Finding{
			Rule: ruleBlocks, Code: "blocks/end-before-start", Severity: SeverityRed, Path: rel,
			Line:    lineAt(content, ends[0]),
			Message: "ownership block malformed: end marker precedes start marker",
			Detail:  fmt.Sprintf("start marker is on line %d", lineAt(content, starts[0])),
		})
	default:
		// Content starts after the start marker's line ends; a same-line
		// block has its content between the markers on that line.
		from := starts[0] + len(start)
		if nl := strings.IndexByte(content[from:ends[0]], '\n'); nl >= 0 {
			from += nl + 1
		}
		return content, [2]int{from, ends[0]}, true
	}
	return content, span, false
}

// markerOffsets returns the byte offset of every non-overlapping occurrence of
// marker in content. Markers are literal strings, matched anywhere in a line:
// indentation or trailing text around a marker does not hide it.
func markerOffsets(content, marker string) []int {
	var offs []int
	for from := 0; ; {
		i := strings.Index(content[from:], marker)
		if i < 0 {
			return offs
		}
		offs = append(offs, from+i)
		from += i + len(marker)
	}
}

// lineAt maps a byte offset to a 1-based line number.
func lineAt(content string, off int) int {
	return 1 + strings.Count(content[:off], "\n")
}
