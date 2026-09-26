## 1. Queue Core

- [x] 1.1 Define task types and versioned payloads in `internal/jobs` with validation
- [x] 1.2 asynq producer adapter with task-id dedup, MaxRetry(5), per-type timeouts
- [x] 1.3 Worker mux: decode/validate, non-retryable malformed payloads, task logging
- [x] 1.4 JobService: NewRecord/Enqueue split, RunJob record sync, GetJob in both stores
- [x] 1.5 report_generate actually generates the report (sync path and job path share formatSummary)

## 2. Runtime

- [x] 2.1 Worker: asynq server with configured concurrency; local polling without Redis
- [x] 2.2 Periodic report scheduler via APP_REPORT_CRON
- [x] 2.3 API wires the producer; compose/env updated with health-gated Redis

## 3. Verification And Docs

- [x] 3.1 Redis end-to-end test: enqueue → worker → done + report; dedup; unknown type
- [x] 3.2 docs/jobs.md rewritten; openspec change registered
