# Command line

```bash
memvet check [flags] [path]
memvet init [--dry-run] [path]
memvet fingerprint [path]
memvet <command> --help
memvet --version
```

`path` defaults to `.`. memvet reads `.memvet.toml` at that path, runs the
rules declared there, and writes findings to stdout.

With no `.memvet.toml` at the path (v0.11), `check` runs the config `init`
would write and reports that first as a YELLOW `config/inferred` finding
naming the rules that ran. It writes nothing. The YELLOW fails the run
under `--strict`, so a CI job that forgot its config cannot pass by
accident. An invalid config is still exit 2; only absence infers.

Real output, from `memvet check --no-color testdata/fixture-broken`:

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
pointers  RED     memory/index.md:10   dead anchor: memory/existing.md has no heading or anchor "nowhere" [pointers/dead-anchor]
    anchors present: #existing-note #overview
tokens    RED     memory/big.md        420 estimated tokens exceeds hard limit of 400 (budget 200) [tokens/over-limit]
junk      YELLOW  notes/scratch.tmp    junk file matches "*.tmp" [junk/match]
pointers  YELLOW  notes/*.md           files glob matched no files [pointers/no-match]
    a stale source glob is pointer coverage that silently never runs
tokens    YELLOW  memory/medium.md     250 estimated tokens exceeds budget of 200 [tokens/over-budget]
tokens    YELLOW  notes/missing/*.md   watch glob matched no files [tokens/no-match]
    a stale watch glob is a budget check that silently never runs
docs: https://github.com/frankbesch/memvet/blob/main/docs/findings.md
memvet: 10 red, 4 yellow (tree 0c61626c461e)
```

A clean repository prints one line:

```text
memvet: clean (3 rules, 4 files checked, tree 244bcef8bd98)
```

## Flags

| Flag | Effect |
|------|--------|
| `--strict` | YELLOW findings also fail the run |
| `--base <ref>` | compare `[append_only]` files against `<ref>` instead of `HEAD` |
| `--changed` | report only findings that touch files changed since `HEAD` (modified, staged, or untracked) |
| `--expect-tree <fp>` | one RED `tree/moved` unless the tree's fingerprint starts with `<fp>` (full or ≥12 hex): the receipt is stale |
| `--format text\|json\|github` | output format, default `text`; `github` emits GitHub Actions annotations |
| `--no-color` | disable ANSI color (also honored: `NO_COLOR`) |
| `-h`, `--help` | usage |

Flags may come before or after the path. An unknown flag anywhere, a flag
without its value, or a second path is a usage error (exit 2) with that
command's help on stderr, never a silent ignore.

Color turns itself off when stdout is not a terminal, so piped and redirected
output is always clean.

`memvet --help` is one screen: the three commands and where to go next.
Each command owns its own help: `memvet check --help` lists the flags
above, `memvet init --help` says what it inspects, `memvet fingerprint
--help` explains the receipt. `memvet help <command>` is the same thing.

## init

`memvet init [path]` inspects the repo and writes `.memvet.toml`. It
prints a report in three tiers: **Enabled** (rules with observed evidence,
written as live sections: `pointers` on whichever of `MEMORY.md`,
`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `COPILOT.md`,
`.github/copilot-instructions.md`, `.cursorrules`, `CONVENTIONS.md` exist
next to a folder of markdown, or on `MEMORY.md` with the sibling root `"."`
for a flat memory folder; `junk` on `.DS_Store` or `*.tmp` seen), **Suggested** (a `decisions.md` by name suggests
`append_only`; two of `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` at the root
suggest `mirrors`; written as commented sections), and **Not inferred**
(everything that needs a choice only you can make, with the choice named).
The suggestion list is closed: nothing else is ever guessed.

`memvet init --dry-run [path]` runs the same inspection, prints the config
it would write to stdout and the report to stderr, and writes nothing. It
works when a config already exists, so discovery can be rerun on a
configured repo. init never overwrites, and there is no `--force`.

`memvet --version` (top-level, before any command) prints the version and
exits 0.

`--changed` is the short-wrap mode: every rule still runs, but only findings
whose file or counterpart changed since `HEAD` are reported, only those
files count as checked, and rules that pay a git call per file skip the
rest. Config-level findings — a glob that matches nothing — still surface,
because dropping one would be a silent skip. Like `--base`, it refuses with
exit 2 outside a git repository rather than quietly running everything.

Tree receipts (`fingerprint`, `--expect-tree`, the `tree …` suffix) are
described in [tree-receipts.md](tree-receipts.md).

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | no RED findings |
| `1` | RED findings, or YELLOW findings with `--strict` |
| `2` | usage error, or a missing/invalid `.memvet.toml` |

Exit 2 is reserved for failures that happen **before** any invariant is
evaluated. It never overlaps with a real result, so CI can tell "your memory
repo is broken" apart from "memvet is misconfigured."

## In GitHub Actions

`--format github` renders every finding as an inline annotation on the pull
request diff — `::error` for RED, `::warning` for YELLOW, `::notice` for
INFO:

```yaml
- run: memvet check --format github --strict .
```

Every finding carries a stable machine code (`pointers/dead-ref`,
`blocks/unterminated`, `tokens/no-match`, ...) used as the annotation title
and present in JSON output. Messages may be reworded between releases; codes
may not. Each code is documented in [docs/findings.md](findings.md) —
what it means and what to do about it. Text output prints the code in
brackets after each message, JSON carries the link as `doc_url`, and a test
fails the build if a code ships without a docs entry.

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
      "doc_url": "https://github.com/frankbesch/memvet/blob/main/docs/findings.md#pointersdead-ref"
    }
  ],
  "summary": {
    "red": 1,
    "yellow": 0,
    "tree": "0951f4c6ddbde0a7f4eae64b7a32e5913ab43bc7958bcace57e91f815eff1ecb"
  }
}
```

`summary.info` appears only when a run carries INFO findings (a detected
log rotation). `summary.tree` (v0.9) is the full tree fingerprint; both are
additive and `schema_version` stays 1.

## When a check cannot run

A rule that cannot evaluate an invariant — an unreadable file, a permission
error mid-walk — emits a **RED** `could not verify:` finding rather than staying
quiet. Unverifiable is treated as failed. A checker that skips a check it was
asked to perform and still reports clean is worse than one that fails loudly.

