# Jobs

The queue is asynq backed by Redis (`APP_REDIS_ADDR`); the worker (`cmd/worker`) runs the task handlers with configured concurrency (`APP_MAX_CONCURRENCY`). Without Redis the API processes jobs in-process for development, and the "Process one" button works; with Redis it is disabled because the worker owns execution.

Job types (definitions in `internal/jobs`, handlers in `internal/platform/queue/worker.go`):

- `user.enrich_batch` — bounded-concurrency external fetches
- `user.report_generate` — analytics snapshot report; also scheduled periodically via `APP_REPORT_CRON`

Operational records: every task mirrors a job row (`GetJob`/`SaveJob`), so the jobs page shows queued/running/done/failed even though Redis executes the work.

Rules:

- Jobs must be idempotent; the asynq task id equals the job id, so a producer retry cannot duplicate a job (`ErrTaskIDConflict`).
- Retries have limits (`MaxRetry(5)`) with asynq's default backoff; malformed payloads return `asynq.SkipRetry` — dead-lettering is asynq's archive after the retry budget is spent.
- Queue concurrency is configured, not hardcoded.
- Job payloads are versioned and validated (`payload_version`), defined once and shared by producer and consumer.
- The store contract suite and the Redis end-to-end test (`make test-integration`) guard the queue behavior.
