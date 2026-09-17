# Development

```bash
go test ./...
go build -o ./memvet .
./memvet check --no-color testdata/fixture-broken   # 10 red, 4 yellow, exit 1
./memvet check --no-color testdata/fixture-clean    # clean, exit 0
./memvet check --no-color testdata/fixture-yellow   # yellow only, exit 0
./memvet check --no-color testdata/fixture-dupids   # 2 red (ids/duplicate), exit 1
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

memvet runs against itself. `.memvet.toml` at this repo's root enables only
the rules that genuinely apply here.

Releases are cut by tagging: push a `vX.Y.Z` tag and
[release.yml](../.github/workflows/release.yml) runs the full test suite, then
goreleaser builds, checksums, and publishes binaries for the platforms CI
tests. `goreleaser release --snapshot --clean --skip=publish` rehearses the
whole pipeline locally without publishing anything.

The cask hook uses GoReleaser's `install_steps` (v2.19 or later). release.yml
runs a preflight step that fails a tag with a named reason when the resolved
GoReleaser is older than that, so a release cannot silently rewrite the tap
cask with Homebrew's deprecated `postflight` stanza. Until v2.19 ships, a
local `goreleaser check` on v2.18 rejects the config; that is expected.

## Roadmap and history

Shipped in v0.11.1: flat-memory inference no longer needs two sibling
notes; `MEMORY.md` at the root with no markdown folder is enough, so the
docs/recipes.md one-liner is true on a one-note or emptied auto-memory
folder. Same finding codes, no new rule.

Shipped in v0.11: `check` without a config runs the inferred config and
says so (`config/inferred`); wider index discovery for `init`; the
`[pointers]` sibling root `"."` for flat memory folders; `action.yml` and
`.pre-commit-hooks.yaml`; the `go run @latest` try-it line.

Shipped in v0.10: flags honored before or after the path; per-command
help; `init` reports Enabled / Suggested / Not inferred and writes only
the sections its evidence supports; `init --dry-run`. Rule expansion is
frozen until v0.10.0 ships (D-143).

Retired 2026-09-11: the `[secrets]` entropy detector. It cannot stay quiet
on a hash-dense memory repo without an allowlist, and an allowlist is a
blind spot `check` can never see through (CLAUDE.md, "smallest rule
surface"). Unprefixed formats belong in `[secrets] patterns` instead.
No roadmap items remain open; rule expansion stays frozen (D-143).

Shipped in v0.9: anchor validation in `[pointers]` (`pointers/dead-anchor`,
reserved since v0.6); the tree fingerprint — `memvet fingerprint`, the
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

Shipped from earlier roadmaps: `memvet init` (v0.4.0), base-ref
`[append_only]` via `--base` (v0.5.0).

Considered from the agent-cohabitation contract and **not** adopted: no-op
commit detection (history hygiene rather than a repo-state invariant, and its
useful form — "this delta is only timestamp churn" — is a content judgment)
and repair-marker validation (detecting *unmarked* degraded output means
validating the output itself, which is semantic linting, a non-goal).

