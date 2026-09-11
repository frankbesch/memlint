# Project: acme

The acme migration moves the billing service from a cron job to an event
queue. Started 2026-08-20. The queue is live in staging; production cutover
waits on the load test.

- 2026-08-21: session 1 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-22: session 2 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-23: session 3 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-24: session 4 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-25: session 5 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-26: session 6 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-27: session 7 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-28: session 8 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-20: session 9 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-21: session 10 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-22: session 11 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-23: session 12 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-24: session 13 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-25: session 14 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-26: session 15 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-27: session 16 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-28: session 17 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-20: session 18 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-21: session 19 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-22: session 20 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-23: session 21 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
- 2026-08-24: session 22 — checked the staging queue, replayed a traffic sample, noted the p95 latency and the retry count for the run.
