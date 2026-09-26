# Performance

The first scale target is 100k users.

Required performance scenarios:

- login burst
- list users with offset pagination
- list users with cursor pagination
- list users with keyset pagination
- create users
- enrichment batch
- report status polling
- dashboard analytics read

Targets should be recorded per feature:

- p95 latency
- p99 latency
- error rate
- throughput
- max concurrency
