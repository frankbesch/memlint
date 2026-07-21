# memlint

An invariant checker for file-based agent memory.

Think `fsck`, not ESLint. AI runtimes like Claude Code and Codex read repos of
markdown as persistent memory and contracts. Those repos drift silently: a
mirrored `CLAUDE.md` gets edited on one side only, an append-only decision log
gets quietly rewritten, a pointer to a note goes dead after a rename. Nothing
fails. The agent just starts working from something that is no longer true.

memlint checks the invariants you declare and reports the ones that broke.
It is **read-only**. It never edits, creates, moves, or deletes a file, and
there is no `--fix` flag to add one.

## Install

```bash
go install github.com/frankbesch/memlint@latest
```

## Usage

```bash
memlint check [flags] [path]
```

`path` defaults to `.`. memlint reads `.memlint.toml` at that path, runs the
rules declared there, and writes findings to stdout.

Real output, from `memlint check --no-color testdata/fixture-broken`:

```text
mirrors   RED     CLAUDE.md          mirrored files differ at byte 115 (line 4, col 19)
    counterpart: docs/CLAUDE.md
    CLAUDE.md is 261 bytes, docs/CLAUDE.md is 258 bytes
mirrors   RED     sync/left/a.md     mirrored files differ at byte 90 (line 4, col 11)
    counterpart: sync/right/a.md
    sync/left/a.md is 115 bytes, sync/right/a.md is 116 bytes
mirrors   RED     sync/left/b.md     present in sync/left but missing from sync/right
    counterpart: sync/right/b.md
pointers  RED     memory/index.md:7  dead reference: memory/missing.md does not exist
pointers  RED     memory/index.md:8  dead reference: docs/nope.md does not exist
junk      YELLOW  notes/scratch.tmp  junk file matches "*.tmp"
tokens    YELLOW  memory/big.md      420 estimated tokens exceeds budget of 200
memlint: 5 red, 2 yellow
```

A clean repository prints one line:

```text
memlint: clean (4 rules, 7 files checked)
```

### Flags

| Flag | Effect |
|------|--------|
| `--strict` | YELLOW findings also fail the run |
| `--format text\|json` | output format, default `text` |
| `--no-color` | disable ANSI color (also honored: `NO_COLOR`) |
| `-h`, `--help` | usage |

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

[pointers]
files = ["memory/index.md"]
roots = ["memory", "docs"]

[junk]
globs = ["*.tmp", ".DS_Store"]

[tokens]
watch = ["memory/*.md"]
budget = 2000
```

memlint rejects any key it does not recognize, along with empty sections,
absolute paths, paths containing `..`, invalid globs, and duplicate entries.
A typo in a config is a check that silently never ran, which is the one failure
mode a checker must not have.

A config with no sections at all is valid. It exits 0 and says
`memlint: clean (no rules enabled)` — never a bare "clean" that could be
mistaken for verification.

## Rules

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

**Scope, precisely:** this compares `HEAD` against the working tree. It is a
working-tree rewrite guard, **not historical immutability enforcement**. Once a
rewrite is committed it becomes the new baseline and this rule goes quiet. See
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

### `[pointers]` — references that must resolve · RED

Extracts repo-path-like references from the listed files, in three passes:
inline code spans, markdown link and image destinations, and bare
whitespace-delimited tokens. A candidate must contain `/`.

A reference is **checked only if its first path segment appears in `roots`**.
That is the whole gate: `memory/notes.md` with `roots = ["memory"]` is memlint's
responsibility, while `reading/daily` is not, because you never told memlint
that `reading` exists.

Skipped entirely:

| Form | Example |
|------|---------|
| URLs | `https://example.com/notes` |
| Date placeholders | `reviews/YYYY-MM-DD-report.html` |
| Angle-bracket placeholders | `memory/<name>.md` |
| Anchors | `memory/notes.md#section` |
| Globs | `memory/*.md` |
| Anything escaping the root | `/etc/passwd`, `../outside.md` |

Anchored references are skipped **whole** in v0.1, rather than having the
fragment stripped and the base file checked. That is the conservative reading,
and changing it is a contract change, not a bug fix — see the roadmap.

Each dead target is reported once per source file, at the line of its first
occurrence. Source files are read whole, so a reference on a line longer than
64 KiB is not silently dropped.

### `[junk]` — files that should not be there · YELLOW

Walks the whole tree, skipping `.git` and not following symlinked directories.
Each glob is matched against both the basename and the root-relative path, so
`.DS_Store` catches every one of them and `scratch/*` catches a directory's
contents. A matching directory is reported once and then pruned.

### `[tokens]` — notes that got too expensive · YELLOW

Estimates one token per four characters, rounded up, counting **runes** rather
than bytes so multibyte content is not overcounted several times over. This is
an estimate and reports itself as one; memlint does not run a tokenizer.

Unlike `[junk]`, watch globs match the root-relative path **only**, never the
basename. A budget is a statement about specific files, and letting `CLAUDE.md`
match every nested `CLAUDE.md` would widen it silently.

### Glob semantics

`*` matches within a single path segment and does not cross `/`. Recursive `**`
is not supported in v0.1 and is rejected at config load rather than being
silently misinterpreted.

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
      "severity": "RED",
      "path": "memory/index.md",
      "related_path": "memory/missing.md",
      "line": 7,
      "message": "dead reference: memory/missing.md does not exist"
    }
  ],
  "summary": {
    "red": 1,
    "yellow": 0
  }
}
```

## When a check cannot run

A rule that cannot evaluate an invariant — an unreadable file, a permission
error mid-walk — emits a **RED** `could not verify:` finding rather than staying
quiet. Unverifiable is treated as failed. A checker that skips a check it was
asked to perform and still reports clean is worse than one that fails loudly.

## Development

```bash
go test ./...
go build -o ./memlint .
./memlint check --no-color testdata/fixture-broken   # 5 red, 2 yellow, exit 1
./memlint check --no-color testdata/fixture-clean    # clean, exit 0
./memlint check --no-color testdata/fixture-yellow   # yellow only, exit 0
```

[testdata/fixture-broken](testdata/fixture-broken) carries five planted RED
defects and two planted YELLOW ones, plus every reference form that must **not**
be reported. The counts are the acceptance test: a rule that starts
over-reporting or quietly stops reporting breaks it.

memlint runs against itself. `.memlint.toml` at this repo's root enables only
the rules that genuinely apply here.

## Roadmap

- **Anchor-aware pointers** — check the base file of `notes.md#section`, and
  eventually the anchor itself.
- **Richer append-only baselines** — compare against the index, an arbitrary
  base ref, or a pull request's merge base, so a *committed* rewrite is caught
  too.
- **`memlint init`** — generate a starter config from what a repo looks like.
- **Recursive `**` globs.**
- **Machine-readable rule docs**, so a finding can link to its own explanation.

## Non-goals for v0.1

Autofix and any form of file mutation. HTML output. Watch mode. Semantic or
content linting — memlint checks mechanical invariants, not whether your notes
are any good. Additional subcommands.

## License

MIT. See [LICENSE](LICENSE).
