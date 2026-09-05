# memlint

[![CI](https://github.com/frankbesch/memlint/actions/workflows/ci.yml/badge.svg)](https://github.com/frankbesch/memlint/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

An invariant checker for file-based agent memory.

Think `fsck`, not ESLint. AI runtimes like Claude Code and Codex read repos of
markdown as persistent memory and contracts. Those repos drift silently: a
mirrored `CLAUDE.md` gets edited on one side only, an append-only decision log
gets quietly rewritten, a pointer to a note goes dead after a rename, an
agent's ownership block loses its end marker and the next run rewrites the
wrong span. Nothing
fails. The agent just starts working from something that is no longer true.

memlint checks the invariants you declare and reports the ones that broke.
`check` is **read-only**: it never edits, creates, moves, or deletes a file,
and there is no `--fix` flag to add one. The single write in the whole tool is
`memlint init`, which creates a starter config once and refuses to overwrite
it.

## Install

```bash
go install github.com/frankbesch/memlint@latest
```

On macOS, Homebrew works too:

```bash
brew install frankbesch/tap/memlint
```

Or download a prebuilt binary for macOS or Linux from the
[releases page](https://github.com/frankbesch/memlint/releases) and verify it
against the release's `checksums.txt`. Whichever route, `memlint --version`
tells you what you got.

## Quick start

From the root of the repo your agent uses as memory:

```bash
memlint init && memlint check
```

`init` inspects the repository and writes a commented `.memlint.toml`,
enabling only rules it found evidence for — an index file like `MEMORY.md`
gets `[pointers]`, observed `.DS_Store` or `*.tmp` files get `[junk]` — and
leaves every other rule as a commented example to adapt. It refuses to
overwrite an existing config, and it is the only memlint command that writes a
file. Review what it generated, then add rules from the [table below](#rules)
as your repo accumulates invariants worth declaring.

## Usage

```bash
memlint check [flags] [path]
memlint init [path]
memlint --version
```

`path` defaults to `.`. memlint reads `.memlint.toml` at that path, runs the
rules declared there, and writes findings to stdout.

Real output, from `memlint check --no-color testdata/fixture-broken`:

```text
blocks    RED     AGENTS.md:3          ownership block unterminated: start marker has no end marker [blocks/unterminated]
    expected "<!-- AGENT:END -->" after it
blocks    RED     docs/generated.md:7  ownership block malformed: duplicate start marker [blocks/duplicate-start]
    first start marker is on line 3
mirrors   RED     CLAUDE.md            mirrored files differ at byte 115 (line 4, col 19) [mirrors/differ]
    counterpart: docs/CLAUDE.md
    CLAUDE.md is 261 bytes, docs/CLAUDE.md is 258 bytes
mirrors   RED     sync/left/a.md       mirrored files differ at byte 90 (line 4, col 11) [mirrors/differ]
    counterpart: sync/right/a.md
    sync/left/a.md is 115 bytes, sync/right/a.md is 116 bytes
mirrors   RED     sync/left/b.md       present in sync/left but missing from sync/right [mirrors/one-sided]
    counterpart: sync/right/b.md
pointers  RED     memory/index.md:7    dead reference: memory/missing.md does not exist [pointers/dead-ref]
pointers  RED     memory/index.md:8    dead reference: docs/nope.md does not exist [pointers/dead-ref]
pointers  RED     memory/index.md:9    dead reference: memory/gone-anchored.md does not exist (referenced as memory/gone-anchored.md#section) [pointers/dead-ref]
junk      YELLOW  notes/scratch.tmp    junk file matches "*.tmp" [junk/match]
pointers  YELLOW  notes/*.md           files glob matched no files [pointers/no-match]
    a stale source glob is pointer coverage that silently never runs
tokens    RED     memory/big.md        420 estimated tokens exceeds hard limit of 400 (budget 200) [tokens/over-limit]
tokens    YELLOW  memory/medium.md     250 estimated tokens exceeds budget of 200 [tokens/over-budget]
tokens    YELLOW  notes/missing/*.md   watch glob matched no files [tokens/no-match]
    a stale watch glob is a budget check that silently never runs
docs: https://github.com/frankbesch/memlint/blob/main/docs/findings.md
memlint: 9 red, 4 yellow
```

A clean repository prints one line:

```text
memlint: clean (6 rules, 9 files checked)
```

### Flags

| Flag | Effect |
|------|--------|
| `--strict` | YELLOW findings also fail the run |
| `--base <ref>` | compare `[append_only]` files against `<ref>` instead of `HEAD` |
| `--format text\|json\|github` | output format, default `text`; `github` emits GitHub Actions annotations |
| `--no-color` | disable ANSI color (also honored: `NO_COLOR`) |
| `-h`, `--help` | usage |

`memlint --version` (top-level, before any command) prints the version and
exits 0.

Flags must come **before** the path. `memlint check . --strict` is refused with
an error rather than silently ignoring `--strict`, which is what the standard
argument parser would otherwise do.

Color turns itself off when stdout is not a terminal, so piped and redirected
output is always clean.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | no RED findings |
| `1` | RED findings, or YELLOW findings with `--strict` |
| `2` | usage error, or a missing/invalid `.memlint.toml` |

Exit 2 is reserved for failures that happen **before** any invariant is
evaluated. It never overlaps with a real result, so CI can tell "your memory
repo is broken" apart from "memlint is misconfigured."

### In GitHub Actions

`--format github` renders every finding as an inline annotation on the pull
request diff — `::error` for RED, `::warning` for YELLOW, `::notice` for
INFO:

```yaml
- run: memlint check --format github --strict .
```

Every finding carries a stable machine code (`pointers/dead-ref`,
`blocks/unterminated`, `tokens/no-match`, ...) used as the annotation title
and present in JSON output. Messages may be reworded between releases; codes
may not. Each code is documented in [docs/findings.md](docs/findings.md) —
what it means and what to do about it. Text output prints the code in
brackets after each message, JSON carries the link as `doc_url`, and a test
fails the build if a code ships without a docs entry.

## Configuration

`.memlint.toml`, at the root you point memlint at. **A section's presence is
what enables its rule.** Omit a section and that rule does not run.

```toml
[mirrors]
pairs = [
  ["CLAUDE.md", "docs/CLAUDE.md"],
  ["sync/left", "sync/right"],
]

[append_only]
files = ["memory/decisions.md"]

[blocks]
files = ["CLAUDE.md", "AGENTS.md"]
start = "<!-- AGENT:START -->"
end = "<!-- AGENT:END -->"

[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = ["openwiki[bot]", "claude"]

[pointers]
files = ["memory/index.md"]
roots = ["memory", "docs"]

[junk]
globs = ["*.tmp", ".DS_Store"]

[tokens]
watch = ["memory/*.md"]
budget = 2000

[ids]
files = ["memory/decisions.md", "memory/archive/*.md"]
```

memlint rejects any key it does not recognize, along with empty sections,
absolute paths, paths containing `..`, invalid globs, and duplicate entries.
A typo in a config is a check that silently never ran, which is the one failure
mode a checker must not have.

A config with no sections at all is valid. It exits 0 and says
`memlint: clean (no rules enabled)` — never a bare "clean" that could be
mistaken for verification.

## Rules

| Rule | Invariant | Severity |
|------|-----------|----------|
| [`[mirrors]`](#mirrors--files-that-must-stay-identical--red) | copies that must stay byte-identical | RED |
| [`[append_only]`](#append_only--logs-that-may-only-grow--red) | logs that may only grow | RED |
| [`[blocks]`](#blocks--ownership-blocks-that-must-stay-well-formed--red) | agent-owned regions that must stay well-formed | RED |
| [`[human_brief]`](#human_brief--files-agents-must-never-write--red) | files no agent may ever have written | RED |
| [`[pointers]`](#pointers--references-that-must-resolve--red) | references that must resolve | RED |
| [`[junk]`](#junk--files-that-should-not-be-there--yellow) | files that should not be there | YELLOW |
| [`[tokens]`](#tokens--notes-that-got-too-expensive--yellow--red) | notes that outgrew their token budget | YELLOW / RED |
| [`[ids]`](#ids--ids-that-must-be-unique--red) | ids that must be unique across files | RED |

### `[mirrors]` — files that must stay identical · RED

Each pair is two files or two directories.

- **Two files:** compared byte for byte. A difference reports the first
  divergent byte offset with its line and column.
- **Two directories:** every `*.md` file beneath them, compared by relative
  path. A file on one side only is a finding; so is a file that differs. `.git`
  is skipped and symlinked directories are not followed.

### `[append_only]` — logs that may only grow · RED

Verifies that the working-tree file still **begins with** the content committed
at git `HEAD`. A violation reports the first divergent line as `was:` / `now:`.

**Scope, precisely:** by default this compares `HEAD` against the working
tree. It is a working-tree rewrite guard, **not historical immutability
enforcement**. Once a rewrite is committed it becomes the new baseline and the
default goes quiet — and a CI checkout's working tree *is* HEAD, so the
default can never fire in CI at all.

`--base <ref>` is what closes that gap: it swaps the baseline from `HEAD` to
any commit `git rev-parse` accepts. In pull-request CI, `--base origin/main`
catches a rewrite that was committed inside the PR, because the PR's files
must still begin with what the base branch holds:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0   # --base needs the base branch's history in the checkout
- run: memlint check --format github --strict --base origin/main .
```

`--base` refuses loudly rather than degrading: an unresolvable ref, a missing
`git`, or a config with no `[append_only]` section is exit 2 at startup — an
explicit baseline demand that cannot be honored must never silently pass. See
[internal/lint/appendonly_test.go](internal/lint/appendonly_test.go), which
tests that limitation explicitly rather than leaving it implied.

The prefix check tolerates a dropped trailing newline, as specified. Note what
that necessarily admits: "drop the final newline, then append" produces bytes
identical to "extend the last line," so the last line may grow. Everything
before it stays strictly immutable — no earlier byte may change and nothing may
be deleted.

No baseline is **YELLOW**, not RED: an untracked file, a directory that is not a
git repository, a repository with no commits, or a missing `git` binary all mean
the invariant could not be established, not that it was violated.

**Rotation.** A log that only grows eventually gets rotated: the oldest
entries move verbatim into an archived volume, and the live file keeps the
rest under a header that now points at the volume. Bytewise that is a
rewrite, and before v0.7 it was RED — so the rotation commit reset the
baseline and memlint verified nothing about the move. Now, when a listed
file no longer begins with its baseline, memlint isolates the **cut span**
(the baseline lines that are gone, header excluded) and looks for it, whole
lines and verbatim, after the header of every *other* listed file that has
no baseline of its own at the ref — untracked, or created after `--base`.
Found: a green **INFO** line instead of the RED, and the destination's
no-baseline YELLOW is withdrawn, because the moved span *is* its baseline:

```text
append_only  INFO    memory/decisions.md:6  rotated → memory/archive/decisions-vol1.md (3 lines moved verbatim) [append_only/rotated]
    baseline lines 6-8 left memory/decisions.md and appear unchanged in memory/archive/decisions-vol1.md
docs: https://github.com/frankbesch/memlint/blob/main/docs/findings.md
memlint: clean (1 rule, 2 files checked)
```

(That is [testdata/fixture-rotated](testdata/fixture-rotated) with its
baseline committed.)

Not found — a moved line altered or dropped, the destination already
committed or not listed — and it is the plain `append_only/rewritten`, as
before. INFO never fails the run, `--strict` included.

The header is the one part of a live log that legitimately changes.
`header_lines = N` marks the first N lines of every listed file — baseline
and working copy alike — as the **only mutable span**; everything after line
N is immutable or must move verbatim. Pick N to cover the header at its
longest, and list the archive volumes too, so they are both rotation
destinations and append-only in their own right:

```toml
[append_only]
files = ["memory/decisions.md", "memory/archive/decisions-vol1.md"]
header_lines = 10   # optional, default 0: the pointer header may change
```

`headers = { "path" = N }` overrides the shared count per file, so an
archive volume with a different header length gets its own window instead
of leaving body lines inside the shared one. Keys must name listed files.

Without `header_lines` a rotation still passes when the header did not
change. A rotation is checkable only while it is uncommitted (or against
`--base`): once committed it is the new baseline like any other change.

### `[blocks]` — ownership blocks that must stay well-formed · RED

Each listed file must contain exactly one well-formed ownership block: one
`start` marker, then one `end` marker. This is the structural half of the
agent-cohabitation convention (an agent that shares a file with humans rewrites
only its own delimited region): whether the agent *stayed inside* its block is
an authorship question the working tree cannot answer, but whether the block it
will rewrite next run is still unambiguous is exactly checkable. A half-deleted
marker means the next run rewrites the wrong span.

Markers are literal single-line strings, matched anywhere in a line, so
indentation or a block opened and closed on the same line both work. Violations
— no markers, end without start, unterminated, duplicate start, duplicate end,
end before start — are RED, **one finding per file**: the first structural
problem is the one a repair has to address before the later ones are
meaningful. A listed file that does not exist is also RED, since the config
declares it carries a block.

The config rejects empty, identical, or multiline markers, and markers that
contain each other, which would make every occurrence of the longer marker also
count as the shorter one.

**Content mirroring.** `mirror = true` additionally requires the content
*between* the markers to be identical in every listed file — the same
agent-owned block embedded in several surfaces. The first listed file is the
reference; a drifted copy is RED `blocks/content-differ` at its first
differing line, and text outside the block may differ freely. A file whose
block is structurally broken gets only its structural finding.

```toml
[blocks]
files = ["surfaces/claude-code-modes.md", "surfaces/claude-ai-instructions.md"]
start = "<!-- modes-block: begin -->"
end = "<!-- modes-block: end -->"
mirror = true
```

Because matching is literal, a file that documents its own markers in prose — an
example that quotes the `start`/`end` strings — has those occurrences counted
too, and will report a duplicate-marker finding. Keep the marker strings out of
the block-carrying file's human-readable examples.

### `[human_brief]` — files agents must never write · RED

The human brief is the file an agent reads for scope and priorities but never
writes — intent and output on opposite sides of a hard line. This rule verifies
that line held: the **full git history** of each listed file must contain no
commit **authored or co-authored** by one of the configured `agent_authors`.
Both the commit author and every `Co-Authored-By:` trailer are checked — the
trailer is the common case, since assisted commits usually land with a human
author and an agent trailer, so matching the author alone would pass the very
commits this rule exists to catch. Matching is by name or email,
case-insensitively but exactly (`claude` does not match `claude reviewer`). A
violation is RED, reported once per file, naming the most recent offending
commit — author or co-author — plus a count of earlier ones.

**Scope, precisely — and deliberately different from `[append_only]`:** that
rule compares only HEAD against the working tree, and a committed rewrite goes
quiet. This rule scans all of history, and that is not scope creep: authorship
is simply not readable from the working tree — a file's bytes do not say who
wrote them — so history is the only place this invariant lives. It also means
an agent-authored commit stays visible after later human commits land on top;
see [internal/lint/humanbrief_test.go](internal/lint/humanbrief_test.go), which
tests both properties explicitly.

What this rule requires of your setup: an agent must be identifiable in commit
metadata — a bot name or bot email in the author field, or in a
`Co-Authored-By:` trailer. If an agent commits as you with no trailer, nothing
in git records that it wrote the file, and the check cannot see it.

For Claude Code specifically, assisted commits land as `Author: <you>` with a
`Co-Authored-By: Claude <model> <noreply@anthropic.com>` trailer. Match on the
**email**, which is stable, not the model name, which changes each release:

```toml
[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = ["noreply@anthropic.com"]
```

Renames are not followed by default; history before a rename belongs to the
old path. `follow_renames = true` walks across renames (`git log --follow`),
so an agent commit to the brief under an earlier name stays visible. No
git repository, no commits, or a file no commit touches is **YELLOW**,
mirroring `[append_only]`: the invariant could not be established, not
violated.

### `[pointers]` — references that must resolve · RED

Extracts repo-path-like references from the listed files, in three passes:
inline code spans, markdown link and image destinations, and bare
whitespace-delimited tokens. A candidate must contain `/`.

A reference is **checked only if its first path segment appears in `roots`**.
That is the whole gate: `memory/notes.md` with `roots = ["memory"]` is memlint's
responsibility, while `reading/daily` is not, because you never told memlint
that `reading` exists.

Anchored references — `memory/notes.md#section` — are checkable since v0.6:
the reference splits at a single `#` and the **base file** is what must exist.
Every skip rule and the roots gate apply to the base, an anchored and a bare
reference to the same file deduplicate to one finding, and whether the anchor
itself resolves to a heading is not yet checked. More than one `#` is not a
path+anchor and is skipped whole.

A `files` entry containing glob metacharacters (`*`, `?`, `[`, `**`) is a
pattern matched against the root-relative path — never the basename, for the
same reason as `[tokens]` watch globs: a source list is a statement about
specific files. A glob matching nothing is **YELLOW** (`pointers/no-match`), a declared
coverage that silently never runs; a missing *literal* entry stays **RED**.

Skipped entirely:

| Form | Example |
|------|---------|
| URLs, with or without fragments | `https://example.com/notes#top` |
| Date placeholders | `reviews/YYYY-MM-DD-report.html` |
| Angle-bracket placeholders | `memory/<name>.md` |
| Multiple `#` | `memory/notes.md#a#b` |
| Globs | `memory/*.md` |
| Anything escaping the root | `/etc/passwd`, `../outside.md` |

Each dead target is reported once per source file, at the line of its first
occurrence. Source files are read whole, so a reference on a line longer than
64 KiB is not silently dropped.

### `[junk]` — files that should not be there · YELLOW

Walks the whole tree, skipping `.git` and not following symlinked directories.
Each glob is matched against both the basename and the root-relative path, so
`.DS_Store` catches every one of them and `scratch/*` catches a directory's
contents. A matching directory is reported once and then pruned.

### `[tokens]` — notes that got too expensive · YELLOW / RED

Estimates one token per four characters, rounded up, counting **runes** rather
than bytes so multibyte content is not overcounted several times over. This is
an estimate and reports itself as one; memlint does not run a tokenizer.

An optional `limit` adds a hard tier above the budget: past `budget` is
YELLOW (`tokens/over-budget`), past `limit` is RED (`tokens/over-limit`).
A file over both tiers reports once, at the worse severity. A set `limit`
must be greater than `budget` — otherwise the yellow band between the
tiers would be empty — and leaving it unset keeps the single-tier
behavior. The budget is the early warning you act on at leisure; the
limit is the line that fails the run even without `--strict`. A soft
tier that is permanently over goes signal-dead — that is what the hard
tier is for.

```toml
[tokens]
watch = ["memory/*.md"]
budget = 2000
limit = 4000   # optional: past this is RED, not YELLOW
```

Unlike `[junk]`, watch globs match the root-relative path **only**, never the
basename. A budget is a statement about specific files, and letting `CLAUDE.md`
match every nested `CLAUDE.md` would widen it silently.

A watch glob that matches no file at all is itself a YELLOW finding
(`tokens/no-match`). A stale glob — a renamed directory, a typo — is a budget
check that silently never runs, which is the same failure mode the config
loader refuses for unknown keys. `[junk]` is exempt: there, matching nothing
is the desired state.

### `[ids]` — ids that must be unique · RED

A decisions log is cited by id. Two sessions that each allocate the next
number in the same minute both write `D-102`, and from then on every
citation of `D-102` is ambiguous. An allocator upstream can prevent new
collisions; only a check on the files can find the ones that landed.

Every id that **opens a line** in any listed file must be unique across all
of them. The default pattern is the decisions-log entry form, delimiter
included:

```toml
[ids]
files = ["memory/decisions.md", "memory/archive/*.md"]
pattern = "^(D-\\d{3}) \\|"   # default: an entry line; the capture group is the id
```

The pattern is matched per line, and only a match starting at column 1 is
an id, whatever the pattern says — a mid-line "see D-001" is a citation. A
duplicate is RED, reported at the *later* occurrence and citing the first:

```text
ids  RED  memory/decisions.md:58  duplicate id D-102: first at memory/decisions.md:43 [ids/duplicate]
```

"First" is source order — literal `files` entries in config order, then
glob matches — then line order, so list the live log before the archive
glob and the live entry is the one a duplicate is measured against. Gaps
are not findings: a withdrawn `D-070` is legitimately absent. Lesson files
or any second numbering would use their own pattern; one section carries
one pattern.

**Known collisions.** An append-only log cannot edit either colliding entry
away, so a reconciled collision would be RED forever. List it under
`known` and its duplicates report as green INFO (`ids/known-duplicate`),
the receipt still visible, the run still clean. A known id that never
collides is YELLOW (`ids/known-unused`): a stale entry would silently
excuse a future collision.

```toml
[ids]
files = ["memory/decisions.md", "memory/archive/*.md"]
known = ["D-102"]   # reconciled by a later ruling; reported as INFO
```

**Dead citations.** `cited_in` lists files whose every cited id must exist
as an entry: dead citations are the id-space version of dead pointers, and
a renumbered ruling is exactly where one hides. `cite_pattern` (default
`\b(D-\d{3})\b`) is matched anywhere in a line. The log's own prose is
not checked unless listed — a log legitimately says "rotate when D-201 is
written". A dead citation is RED `ids/dead-cite`.

**Order.** `ordered = true` requires each file's entries to be
non-decreasing by the number in the id, so a paste into the middle of an
append-only log — invisible to the prefix check once committed — is RED
`ids/out-of-order`. Gaps and known duplicates are fine.

```toml
[ids]
files = ["memory/decisions.md", "memory/archive/*.md"]
known = ["D-102"]
cited_in = ["CLAUDE.md", "DASHBOARD.md", "memory/handoff.md"]
ordered = true
```

`files` resolves like `[pointers]` files: globs match the root-relative
path only, a glob matching nothing is YELLOW (`ids/no-match`), and a
missing literal entry is RED (`ids/missing-source`).

**Why the delimiter is in the default:** entries that wrap by hand produce
continuation lines, and one can start with a cited id — `D-102). (2) the
next step…` at column 1. A bare `^(D-\\d{3})` reads that as a second
entry; the first real run did exactly that, eleven times. Requiring
` |` after the id means only entry lines match. The trade is explicit: an
entry whose first line lacks the delimiter is not an id either, so keep the
entry format uniform. A log with another shape sets its own pattern.

### Glob semantics

`*` and `?` match within a single path segment and never cross `/`. A
whole-segment `**` matches zero or more directories: `memory/**/*.md` covers
`memory/a.md` and `memory/deep/er/a.md`, `memory/**` covers everything
under `memory/`, `**/*.tmp` covers the tree. `**` inside a segment (`a**b`)
is rejected at config load rather than silently misread.

### Path safety

Every configured path and every extracted reference is resolved against the
target root and must stay inside it. Absolute paths, `..` traversal, and
symlinks whose target leaves the root are refused. Recursive walks never follow
symlinked directories, so a link cannot widen the scope of a check.

## JSON output

`--format json` emits a stable document, never colored, with findings in
deterministic order:

```json
{
  "schema_version": 1,
  "findings": [
    {
      "rule": "pointers",
      "code": "pointers/dead-ref",
      "severity": "RED",
      "path": "memory/index.md",
      "related_path": "memory/missing.md",
      "line": 7,
      "message": "dead reference: memory/missing.md does not exist",
      "doc_url": "https://github.com/frankbesch/memlint/blob/main/docs/findings.md#pointersdead-ref"
    }
  ],
  "summary": {
    "red": 1,
    "yellow": 0
  }
}
```

`summary.info` appears only when a run carries INFO findings (a detected
log rotation), so output for a repository with nothing to report is
unchanged from earlier releases.

## When a check cannot run

A rule that cannot evaluate an invariant — an unreadable file, a permission
error mid-walk — emits a **RED** `could not verify:` finding rather than staying
quiet. Unverifiable is treated as failed. A checker that skips a check it was
asked to perform and still reports clean is worse than one that fails loudly.

## Development

```bash
go test ./...
go build -o ./memlint .
./memlint check --no-color testdata/fixture-broken   # 9 red, 4 yellow, exit 1
./memlint check --no-color testdata/fixture-clean    # clean, exit 0
./memlint check --no-color testdata/fixture-yellow   # yellow only, exit 0
./memlint check --no-color testdata/fixture-dupids   # 2 red (ids/duplicate), exit 1
```

[testdata/fixture-broken](testdata/fixture-broken) carries nine planted RED
defects and four planted YELLOW ones, plus every reference form that must **not**
be reported. The counts are the acceptance test: a rule that starts
over-reporting or quietly stops reporting breaks it.
[testdata/fixture-rotated](testdata/fixture-rotated) is an append-only log
mid-rotation; because a committed file always equals its own HEAD, its test
builds the git state around it rather than checking the tree as it sits.
[testdata/fixture-dupids](testdata/fixture-dupids) plants exactly two id
collisions next to a gap and a mid-line mention that must stay silent.

memlint runs against itself. `.memlint.toml` at this repo's root enables only
the rules that genuinely apply here.

Releases are cut by tagging: push a `vX.Y.Z` tag and
[release.yml](.github/workflows/release.yml) runs the full test suite, then
goreleaser builds, checksums, and publishes binaries for the platforms CI
tests. `goreleaser release --snapshot --clean --skip=publish` rehearses the
whole pipeline locally without publishing anything.

## Roadmap

In priority order:

Shipped in v0.8: recursive `**` globs in every glob-taking key; `[blocks]`
content mirroring (`mirror = true`); rename-aware `[human_brief]`
(`follow_renames = true`); per-file `headers` for `[append_only]`.

Shipped from this list: anchor-aware `[pointers]` checking and glob support
in `[pointers]` files (v0.6); self-describing findings — every code documented
in [docs/findings.md](docs/findings.md), linked from text output and as
`doc_url` in JSON, with no `explain` subcommand by ruling; two-tier
`[tokens]`; rotation-aware `[append_only]` with `header_lines`; the `[ids]`
rule (all v0.7.0).

Shipped from earlier roadmaps: `memlint init` (v0.4.0), base-ref
`[append_only]` via `--base` (v0.5.0).

Considered from the agent-cohabitation contract and **not** adopted: no-op
commit detection (history hygiene rather than a repo-state invariant, and its
useful form — "this delta is only timestamp churn" — is a content judgment)
and repair-marker validation (detecting *unmarked* degraded output means
validating the output itself, which is semantic linting, a non-goal).

## Non-goals

Autofix and any form of repair — `check` never mutates a file, and `init`, the
one write in the whole tool, only ever creates a `.memlint.toml` that did not
exist. HTML output. Watch mode. Semantic or content linting — memlint checks
mechanical invariants, not whether your notes are any good.

## License

MIT. See [LICENSE](LICENSE).
