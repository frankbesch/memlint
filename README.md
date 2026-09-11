# memlint

[![CI](https://github.com/frankbesch/memlint/actions/workflows/ci.yml/badge.svg)](https://github.com/frankbesch/memlint/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Integrity checks for file-based AI agent state: memory, instructions,
decisions, and contracts.

Think `fsck`, not ESLint. memlint verifies properties you declare must stay
true across a repo of markdown that an agent runtime reads as memory. It does
not judge prose, and it is not a memory store or retrieval system.

`check` is **read-only**. It never edits, creates, moves, or deletes a file,
and there is no `--fix`. The one write in the whole tool is `memlint init`,
which creates a starter config once and refuses to overwrite it.

## The failure mode

Runtimes like Claude Code and Codex load `CLAUDE.md`, `MEMORY.md`, and a
folder of notes into context on every run. Those files drift silently:

- `MEMORY.md` still points at a note that was renamed last week.
- An append-only decision log was quietly rewritten.
- `CLAUDE.md` and `AGENTS.md` are supposed to be identical and no longer are.

Nothing fails. The agent just starts working from something that is no longer
true. memlint turns each of those into a RED finding with a file, a line, and
a stable code.

## 60-second example

[examples/broken](examples/broken) is a three-file memory repo with two
defects. Its config:

```toml
[pointers]
files = ["MEMORY.md"]
roots = ["memory"]

[tokens]
watch = ["MEMORY.md", "memory/*.md"]
budget = 400
```

`memlint check examples/broken` prints:

<!-- examples/broken output: kept identical to the real run by TestReadmeOutputMatchesExample -->
```text
pointers  RED     MEMORY.md:6     dead reference: memory/preferences.md does not exist [pointers/dead-ref]
tokens    YELLOW  memory/acme.md  794 estimated tokens exceeds budget of 400 [tokens/over-budget]
docs: https://github.com/frankbesch/memlint/blob/main/docs/findings.md
memlint: 1 red, 1 yellow (tree 22f124e8d9eb)
```

RED means a declared invariant is broken and the run exits 1. YELLOW means
something needs attention but does not fail the run unless you pass
`--strict`. Every code links to a plain-English entry in
[docs/findings.md](docs/findings.md).

## Install

```bash
brew install frankbesch/tap/memlint
```

```bash
go install github.com/frankbesch/memlint@latest
```

Or try it once without installing anything, on the repo you are in:

```bash
go run github.com/frankbesch/memlint@latest check .
```

With no `.memlint.toml` yet, `check` runs what it can infer from the tree
and says so in its first line.

Or download a binary for macOS or Linux from the
[releases page](https://github.com/frankbesch/memlint/releases) and verify it
against `checksums.txt`. `memlint --version` tells you what you got.

## Quick start

From the root of the repo your agent uses as memory:

```bash
memlint init
memlint check
```

`init` inspects the repo, writes a `.memlint.toml` that enables only the
rules it found evidence for, and reports what it enabled, what it only
suggests, and what it refused to guess. `memlint init --dry-run` shows the
config without writing it. Review it, then add rules from the table below as your repo accumulates invariants worth declaring. Ready-made configs
for common layouts are in [docs/recipes.md](docs/recipes.md).

## What can it protect?

| I need to make sure that… | Rule |
|---|---|
| references between memory files still resolve | [`pointers`](docs/rules.md#pointers--references-that-must-resolve--red) |
| the decision log was only ever appended to | [`append_only`](docs/rules.md#append_only--logs-that-may-only-grow--red) |
| copies of the agent instructions stay identical | [`mirrors`](docs/rules.md#mirrors--files-that-must-stay-identical--red) |
| an agent-owned block keeps its start and end markers | [`blocks`](docs/rules.md#blocks--ownership-blocks-that-must-stay-well-formed--red) |
| a human-written brief was never written by an agent | [`human_brief`](docs/rules.md#human_brief--files-agents-must-never-write--red) |
| decision ids are unique, cited, and in order | [`ids`](docs/rules.md#ids--ids-that-must-be-unique--red) |
| memory files stay within a token budget | [`tokens`](docs/rules.md#tokens--notes-that-got-too-expensive--yellow--red) |
| "last verified" stamps keep up with edits | [`stamps`](docs/rules.md#stamps--last-verified-dates-that-must-keep-up--yellow) |
| scratch files do not creep into the repo | [`junk`](docs/rules.md#junk--files-that-should-not-be-there--yellow) |
| credential-shaped strings never land in memory | [`secrets`](docs/rules.md#secrets--credentials-that-must-not-be-there--red) |

Ten rules, five ideas:

| Family | Rules | Protects against |
|---|---|---|
| Referential integrity | `pointers`, `ids` | dangling or ambiguous references |
| Mutation and provenance | `append_only`, `human_brief` | rewritten history, wrong author |
| Replication and ownership | `mirrors`, `blocks` | copies that disagree, unsafe shared regions |
| Freshness and capacity | `stamps`, `tokens` | stale or bloated context |
| Hygiene tripwires | `junk`, `secrets` | things that should not be in the tree |

A section's presence in `.memlint.toml` is what enables its rule. Full key
reference: [docs/rules.md](docs/rules.md).

## In CI

The shortest form is the action, which fetches the released binary and
runs `check --format github`:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: frankbesch/memlint@v0.11.0
  with:
    strict: true
    base: ${{ github.event.pull_request.base.sha }}
```

A complete pull-request workflow using `go install` instead is in
[.github/examples/memlint.yml](.github/examples/memlint.yml). The core of it:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: actions/setup-go@v5
  with:
    go-version: stable
- run: go install github.com/frankbesch/memlint@latest
- run: memlint check --format github --strict --base "${{ github.event.pull_request.base.sha }}" .
```

`--format github` renders each finding as an inline annotation on the diff.
`--base` matters for `append_only`: a fresh checkout equals its own HEAD, so
without a base the log has nothing to be compared against. `fetch-depth: 0`
makes that base reachable.

## Tree receipts

Every `check` summary ends with a fingerprint of the exact tree it judged.
A later run can be held to it with `--expect-tree`, which turns a tree that
changed in between into one RED finding. memlint stores nothing; the caller
keeps the receipt. Details in [docs/tree-receipts.md](docs/tree-receipts.md).

## Documentation

- [Configuration](docs/configuration.md): the `.memlint.toml` format and what it rejects.
- [Rules](docs/rules.md): every rule, its keys, what it proves and what it cannot.
- [Recipes](docs/recipes.md): copy-and-paste configs for common memory layouts, each a runnable example.
- [Command line](docs/cli.md): flags, exit codes, JSON and GitHub output.
- [Finding codes](docs/findings.md): what each code means and what to do.
- [Tree receipts](docs/tree-receipts.md): fingerprints and `--expect-tree`.
- [Development](docs/development.md): tests, fixtures, releases, roadmap.
- [SPEC.md](SPEC.md): the versioned build log, for design provenance.

## What memlint does not do

- Judge whether a memory is correct, useful, or well written.
- Decide what an agent should remember.
- Prove agent authorship when git metadata does not identify the agent.
- Replace a full secret scanner; `[secrets]` is a last-mile tripwire on the working tree.
- Repair anything. `check` never writes, and `init` only creates a config that did not exist.
- Watch mode, HTML output, or a hosted service.

## Contributing

Behavior changes start as a SPEC.md addendum with gates; see
[CONTRIBUTING.md](CONTRIBUTING.md). Bug reports and proposed invariants are
welcome as issues.

## License

MIT. See [LICENSE](LICENSE).
