# Rules

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
| [`[stamps]`](#stamps--last-verified-dates-that-must-keep-up--yellow) | last-verified dates that must keep up with the file | YELLOW |
| [`[secrets]`](#secrets--credentials-that-must-not-be-there--red) | credential-shaped text that must not be there | RED |

## `[mirrors]` — files that must stay identical · RED

Each pair is two files or two directories.

- **Two files:** compared byte for byte. A difference reports the first
  divergent byte offset with its line and column.
- **Two directories:** every `*.md` file beneath them, compared by relative
  path. A file on one side only is a finding; so is a file that differs. `.git`
  is skipped and symlinked directories are not followed.

## `[append_only]` — logs that may only grow · RED

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
[internal/lint/appendonly_test.go](../internal/lint/appendonly_test.go), which
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

(That is [testdata/fixture-rotated](../testdata/fixture-rotated) with its
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

## `[blocks]` — ownership blocks that must stay well-formed · RED

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

## `[human_brief]` — files agents must never write · RED

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
see [internal/lint/humanbrief_test.go](../internal/lint/humanbrief_test.go), which
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

## `[pointers]` — references that must resolve · RED

Extracts repo-path-like references from the listed files, in three passes:
inline code spans, markdown link and image destinations, and bare
whitespace-delimited tokens. A candidate must contain `/`.

A reference is **checked only if its first path segment appears in `roots`**.
That is the whole gate: `memory/notes.md` with `roots = ["memory"]` is memlint's
responsibility, while `reading/daily` is not, because you never told memlint
that `reading` exists.

**The root `"."`** (v0.11) is different in kind: it turns on a second pass
over markdown link and image destinations only, for destinations with no
slash at all, which resolve against the source file's own directory. It
exists for flat memory folders — Claude Code's auto-memory is `MEMORY.md`
beside its notes, indexed as `[Title](note.md)` — where no first-segment
root could ever match. Inline code spans and bare tokens are excluded from
that pass on purpose: `a.md` in prose is a word, not a claim. `"."` may sit
beside named roots; each pass is gated by its own rule. `memlint init`
infers it when `MEMORY.md` sits at the root and no markdown folder exists
(since v0.11.1 the note count is not a condition; v0.11.0 required two or
more sibling notes, which left a young or emptied folder unchecked).

Anchored references — `memory/notes.md#section` — split at a single `#`.
The **base file** must exist (since v0.6; a dead base is one `pointers/dead-ref`
however many anchors point at it), and since v0.9 the **anchor** must resolve
when the base is markdown: a heading whose GitHub-style slug equals it, a
`{#custom-id}` suffix, or an explicit `<a id>`/`<a name>` — otherwise **RED**
`pointers/dead-anchor`, with the anchors the file does expose in the detail
line. Headings inside fenced code do not count, and non-markdown targets are
never anchor-checked. More than one `#` is not a path+anchor and is skipped
whole.

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

## `[junk]` — files that should not be there · YELLOW

Walks the whole tree, skipping `.git` and not following symlinked directories.
Each glob is matched against both the basename and the root-relative path, so
`.DS_Store` catches every one of them and `scratch/*` catches a directory's
contents. A matching directory is reported once and then pruned.

## `[tokens]` — notes that got too expensive · YELLOW / RED

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

## `[ids]` — ids that must be unique · RED

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

## `[stamps]` — last-verified dates that must keep up · YELLOW

A note that says "Last verified: 2026-08-01" and was edited on 2026-09-01
is carrying expired evidence. Each listed file must carry a stamp
(`Last verified: YYYY-MM-DD` by default; `pattern` must capture the date)
no older than `max_age_days` relative to the file's **last change** — its
last commit date, or now if it has uncommitted edits — not relative to
today, so an old note with an equally old stamp is fine.

```toml
[stamps]
files = ["portfolio/*.md", "memory/surfaces.md"]
max_age_days = 30
```

Stale is YELLOW (`stamps/stale`) on the stamp line; a listed file with no
stamp at all is RED (`stamps/missing`); no git history is YELLOW.

## `[secrets]` — credentials that must not be there · RED

A card number once rode three commits deep in a memory repo before anyone
questioned the push. This is the working-tree tripwire: files matching
`globs` are scanned for credential-shaped text — AWS keys, GitHub tokens,
Anthropic and OpenAI keys, Slack tokens, private-key blocks, and card
numbers (13–19 digits that pass the Luhn check, so order and phone numbers
stay quiet). `patterns` adds the repo's own. The value is **never printed**.

```toml
[secrets]
globs = ["**/*.md"]
patterns = ["FB-SECRET-\\d+"]   # optional extras
```

Run it with `--changed` before committing and a secret cannot reach history.

## Glob semantics

`*` and `?` match within a single path segment and never cross `/`. A
whole-segment `**` matches zero or more directories: `memory/**/*.md` covers
`memory/a.md` and `memory/deep/er/a.md`, `memory/**` covers everything
under `memory/`, `**/*.tmp` covers the tree. `**` inside a segment (`a**b`)
is rejected at config load rather than silently misread.

## Path safety

Every configured path and every extracted reference is resolved against the
target root and must stay inside it. Absolute paths, `..` traversal, and
symlinks whose target leaves the root are refused. Recursive walks never follow
symlinked directories, so a link cannot widen the scope of a check.

