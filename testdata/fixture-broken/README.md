# fixture-broken

A deliberately broken agent-memory repository. `memlint check` must report
exactly **7 RED** and **3 YELLOW** findings here, and exit 1.

| # | Severity | Rule | Planted defect |
|---|----------|----------|----------------|
| 1 | RED | blocks | `AGENTS.md` opens an ownership block and never closes it |
| 2 | RED | blocks | `docs/generated.md` carries a duplicate start marker |
| 3 | RED | mirrors | `CLAUDE.md` differs from `docs/CLAUDE.md` |
| 4 | RED | mirrors | `sync/left/a.md` differs from `sync/right/a.md` |
| 5 | RED | mirrors | `sync/left/b.md` has no counterpart in `sync/right/` |
| 6 | RED | pointers | `memory/missing.md` does not exist |
| 7 | RED | pointers | `docs/nope.md` does not exist |
| 8 | YELLOW | junk | `notes/scratch.tmp` matches `*.tmp` |
| 9 | YELLOW | tokens | `memory/big.md` exceeds the 200-token budget |
| 10 | YELLOW | tokens | watch glob `notes/missing/*.md` matches no files |

`memory/index.md` also carries every reference form that must **not** produce a
finding. The load-bearing one is `memory/gone-anchored.md#section`: that file
does not exist and is referenced nowhere else, so if anchor-skipping ever
regresses this fixture reports 6 RED and the test fails.

Nothing in this directory is a real memory repo. Do not copy it as a template.
