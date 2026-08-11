# fixture-broken

A deliberately broken agent-memory repository. `memlint check` must report
exactly **8 RED** and **4 YELLOW** findings here, and exit 1.

| # | Severity | Rule | Planted defect |
|---|----------|----------|----------------|
| 1 | RED | blocks | `AGENTS.md` opens an ownership block and never closes it |
| 2 | RED | blocks | `docs/generated.md` carries a duplicate start marker |
| 3 | RED | mirrors | `CLAUDE.md` differs from `docs/CLAUDE.md` |
| 4 | RED | mirrors | `sync/left/a.md` differs from `sync/right/a.md` |
| 5 | RED | mirrors | `sync/left/b.md` has no counterpart in `sync/right/` |
| 6 | RED | pointers | `memory/missing.md` does not exist |
| 7 | RED | pointers | `docs/nope.md` does not exist |
| 8 | RED | pointers | `memory/gone-anchored.md#section` — the base file does not exist |
| 9 | YELLOW | pointers | files glob `notes/*.md` matches no files |
| 10 | YELLOW | junk | `notes/scratch.tmp` matches `*.tmp` |
| 11 | YELLOW | tokens | `memory/big.md` exceeds the 200-token budget |
| 12 | YELLOW | tokens | watch glob `notes/missing/*.md` matches no files |

`memory/index.md` also carries every reference form that must **not** produce
a finding. Two are load-bearing since v0.6 made anchored references checkable:
`memory/multi-hash.md#a#b` (more than one `#` is not a path+anchor) and the
URL-with-fragment — neither target exists, so if either skip regresses this
fixture over-reports and the test fails. `memory/missing.md#section` pins
dedup: the anchored form and the bare ref above it are one finding, not two.

Nothing in this directory is a real memory repo. Do not copy it as a template.
