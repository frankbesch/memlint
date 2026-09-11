# Recipes

Copy-and-paste configs for common memory layouts. Each one is a real
directory under [examples/](../examples) that CI runs on every commit, so the
config shown here is known to load and to produce the stated result. Adapt
paths and budgets to your repo; the point is which invariants belong
together.

## Basic agent memory

An index file points at notes. The notes must exist and stay small, and
scratch files must not creep in. Directory: [examples/minimal](../examples/minimal).

```toml
[pointers]
files = ["MEMORY.md"]
roots = ["memory"]

[tokens]
watch = ["MEMORY.md", "memory/*.md"]
budget = 400

[junk]
globs = [".DS_Store", "*.tmp"]
```

Result: `memlint: clean (3 rules, 4 files checked, …)`.

For a Claude Code repo, add `CLAUDE.md` to `files` and any folder that
`MEMORY.md` links into to `roots`. `memlint init` does exactly this when it
finds those files.

## Claude Code auto-memory

Claude Code keeps per-project memory outside the repo, as `MEMORY.md`
beside its notes, indexed one line each as `[Title](note.md) — hook`. There
is no folder to name as a root, so the sibling root `"."` does the work:

```toml
[pointers]
files = ["MEMORY.md"]
roots = ["."]
```

That is also what `check` infers when there is no config, so the one-line
version needs nothing written:

```bash
memlint check ~/.claude/projects/<project-slug>/memory
```

A renamed or deleted note is a RED `pointers/dead-ref` at the index line
that still names it. Prose that happens to mention `a.md` is never a
reference; only link destinations count.

## Durable decision log

Entries may be appended, never rewritten, and every id is unique and in
order. Directory: [examples/decision-log](../examples/decision-log).

```toml
[append_only]
files = ["memory/decisions.md"]

[ids]
files = ["memory/decisions.md"]
ordered = true
```

Result: clean once the log is committed. Before the first commit,
`append_only` reports YELLOW `append_only/no-baseline`, because there is no
HEAD to compare against. In CI, pass `--base` so the comparison is against
the pull request's base rather than the checkout's own HEAD.

The default id shape is `D-### | …` at column 1; set `pattern` for another
form. Add `cited_in` to require that every id mentioned in another file
exists in the log.

## Shared instructions across runtimes

One set of instructions read by two runtimes as `CLAUDE.md` and `AGENTS.md`.
The copies must stay byte-identical, and the agent-owned block inside them
must keep its markers. Directory:
[examples/shared-instructions](../examples/shared-instructions).

```toml
[mirrors]
pairs = [
  ["CLAUDE.md", "AGENTS.md"],
]

[blocks]
files = ["CLAUDE.md", "AGENTS.md"]
start = "<!-- AGENT:START -->"
end = "<!-- AGENT:END -->"
```

Result: `memlint: clean (2 rules, 2 files checked, …)`.

## A broken repo, for reference

[examples/broken](../examples/broken) is the basic recipe applied to a repo
with one dead reference and one oversized note. It exits 1 with 1 red and 1
yellow; the README quotes its output verbatim, and a test keeps that quote
identical to the real run.

## Before every commit

With the [pre-commit](https://pre-commit.com) framework, memlint builds
itself from this repository at the pinned release and runs `check
--changed` on the files you are about to commit:

```yaml
- repo: https://github.com/frankbesch/memlint
  rev: v0.11.0
  hooks:
    - id: memlint
```

Set `args: []` to run the full check instead of only changed files.
