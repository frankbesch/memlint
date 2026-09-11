# Project: acme

The acme migration moves the billing service from a cron job to an event
queue. Started 2026-08-20. The queue is live in staging; production cutover
waits on the load test.

Next: run the load test against staging with last month's traffic replay.
