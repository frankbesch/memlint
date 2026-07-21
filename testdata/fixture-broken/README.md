# fixture-broken

A deliberately broken agent-memory repository. `memlint check` must report
exactly **5 RED** and **2 YELLOW** findings here, and exit 1.

| # | Severity | Rule | Planted defect |
|---|----------|----------|----------------|
| 1 | RED | mirrors | `CLAUDE.md` differs from `docs/CLAUDE.md` |
| 2 | RED | mirrors | `sync/left/a.md` differs from `sync/right/a.md` |
| 3 | RED | mirrors | `sync/left/b.md` has no counterpart in `sync/right/` |
| 4 | RED | pointers | `memory/missing.md` does not exist |
| 5 | RED | pointers | `docs/nope.md` does not exist |
| 6 | YELLOW | junk | `notes/scratch.tmp` matches `*.tmp` |
| 7 | YELLOW | tokens | `memory/big.md` exceeds the 200-token budget |

`memory/index.md` also carries every reference form that must **not** produce a
finding. The load-bearing one is `memory/gone-anchored.md#section`: that file
does not exist and is referenced nowhere else, so if anchor-skipping ever
regresses this fixture reports 6 RED and the test fails.

Nothing in this directory is a real memory repo. Do not copy it as a template.
