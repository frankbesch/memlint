# memlint — agent contract

This file is the maintainer's contract with AI coding agents working in this
repo. Users want README.md; contributors want CONTRIBUTING.md.

memlint is Frank Besch's invariant checker for file-based agent memory
(Go, MIT, `github.com/frankbesch/memlint`). Think `fsck`, not ESLint. It
reads a repo of markdown that AI runtimes treat as memory and reports the
declared invariants that broke. README.md is the user manual and is
authoritative on behavior; SPEC.md is the build log (versioned addenda);
docs/findings.md is the finding-code reference.

## Hard rules

1. `check` is read-only forever. Never add a `--fix`, an autofix path, or
   any write outside `memlint init`. Unverifiable is RED, never silent.
2. Every behavior change starts as a SPEC.md addendum with numbered gates
   (G1–G5 pattern: tests, gofmt/vet, fixture counts, self-check on FBOS,
   receipt). A gate is a command, its exit status, and the matching output
   line. Unrun gate = not shipped. Product rulings live in FBOS
   (`~/Documents/promptkits/memory/decisions.md`, D-###); cite the D-### in
   the addendum header and the commit message. Never write FBOS state here.
3. Fixture counts are the acceptance test. Changing a planted defect changes
   docs/development.md and the addendum in the same commit.
4. Finding codes (`rule/code`) are stable once released; messages may be
   reworded, codes may not.
5. Dependencies: BurntSushi/toml and golang.org/x/term only. stdlib `flag`,
   no cobra.
6. This repo is a consumer of itself and of FBOS: `~/go/bin/memlint` is the
   binary the FBOS wrap and push gate run. After a shipped change, reinstall
   it (`go install .`) and say so; a stale wrap binary is a silent gate.

## Commands

```bash
go test ./...                                   # must be green
gofmt -l . && go vet ./...                      # both empty / clean
go build -o ./memlint .
./memlint check --no-color testdata/fixture-broken   # 10 red, 4 yellow, exit 1
./memlint check --no-color testdata/fixture-clean    # clean, exit 0
./memlint check --strict ~/Documents/promptkits      # self-check on FBOS
go install .                                    # refresh ~/go/bin/memlint
goreleaser release --snapshot --clean --skip=publish # rehearse a release
```

Releases: push a `vX.Y.Z` tag; release.yml runs tests and goreleaser. Tag
only after the gates pass and the FBOS decision is recorded.

## Working here

- Local hooks in `.claude/settings.json` filter long build/test output
  through `scripts/output-filter-hook.sh`; full logs land beside the command
  file. Read the log before declaring a gate passed.
- Review, explain, and diagnose requests get findings, not changes. Build
  and fix requests get in-scope changes plus the gate run.
- Prefer the smallest rule surface that catches the real defect; a rule that
  needs an allowlist to stay quiet on the fixtures is not ready (the
  `[secrets]` entropy detector was retired 2026-09-11 for exactly this
  reason; unprefixed formats go in `[secrets] patterns`).

## Voice line

When spoken to through backtalk, the assistant is FBOS in this repo: answer
about memlint's state, rules, gates, and roadmap; route rulings and wraps to
the typed promptkits session.
