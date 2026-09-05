# fixture-dupids

A decisions log with two id collisions. `memlint check` must report exactly
**2 RED** `ids/duplicate` findings here and exit 1.

| # | Severity | Rule | Planted defect |
|---|----------|------|----------------|
| 1 | RED | ids | `D-050` is in `memory/archive/decisions-vol1.md` and again in `memory/decisions.md` |
| 2 | RED | ids | `D-102` appears twice in `memory/decisions.md` (two sessions, same minute) |

Planted non-findings: `D-002` is absent (a gap is not a finding), and the
header's "See D-001" is a mid-line mention, not an id — only a match at
column 1 counts.

The live log is listed before the archive glob, so its `D-050` is "first"
and the volume's occurrence is reported as the duplicate.

Nothing in this directory is a real memory repo. Do not copy it as a template.
