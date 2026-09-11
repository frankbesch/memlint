# Decisions

Append only. One entry per line, `D-### | date | decision | why`.

D-001 | 2026-08-20 | Move billing to an event queue | the cron job double-charged twice in July
D-002 | 2026-08-28 | Cut over only after a load test with replayed traffic | staging has never seen production volume
D-003 | 2026-09-05 | Keep the cron job runnable for 30 days after cutover | rollback must not need a deploy
