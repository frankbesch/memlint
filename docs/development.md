# Development

```bash
go test ./...
go build -o ./memlint .
./memlint check --no-color testdata/fixture-broken   # 10 red, 4 yellow, exit 1
./memlint check --no-color testdata/fixture-clean    # clean, exit 0
./memlint check --no-color testdata/fixture-yellow   # yellow only, exit 0
./memlint check --no-color testdata/fixture-dupids   # 2 red (ids/duplicate), exit 1
```

[testdata/fixture-broken](../testdata/fixture-broken) carries ten planted RED
defects and four planted YELLOW ones, plus every reference form that must **not**
be reported. The counts are the acceptance test: a rule that starts
over-reporting or quietly stops reporting breaks it.
[testdata/fixture-rotated](../testdata/fixture-rotated) is an append-only log
mid-rotation; because a committed file always equals its own HEAD, its test
builds the git state around it rather than checking the tree as it sits.
[testdata/fixture-dupids](../testdata/fixture-dupids) plants exactly two id
collisions next to a gap and a mid-line mention that must stay silent.

memlint runs against itself. `.memlint.toml` at this repo's root enables only
the rules that genuinely apply here.

Releases are cut by tagging: push a `vX.Y.Z` tag and
[release.yml](../.github/workflows/release.yml) runs the full test suite, then
goreleaser builds, checksums, and publishes binaries for the platforms CI
tests. `goreleaser release --snapshot --clean --skip=publish` rehearses the
whole pipeline locally without publishing anything.

## Roadmap and history

v0.9 futures, in priority order:

1. **`[secrets]` entropy detector** — catch high-entropy strings the
   shape-based patterns miss, with an allowlist for fixtures.

Shipped in v0.9: anchor validation in `[pointers]` (`pointers/dead-anchor`,
reserved since v0.6); the tree fingerprint — `memlint fingerprint`, the
`tree …` receipt in every summary, `--expect-tree`, and `summary.tree`.

Shipped in v0.8: recursive `**` globs in every glob-taking key; `[blocks]`
content mirroring (`mirror = true`); rename-aware `[human_brief]`
(`follow_renames = true`); per-file `headers` for `[append_only]`; `[ids]`
`cited_in` and `ordered`; the `[stamps]` and `[secrets]` rules; `check
--changed`.

Shipped from this list: anchor-aware `[pointers]` checking and glob support
in `[pointers]` files (v0.6); self-describing findings — every code documented
in [docs/findings.md](findings.md), linked from text output and as
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

