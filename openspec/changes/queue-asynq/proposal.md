## Why

Jobs were processed only in the process that received the request, and the separate worker binary polled its own empty store. Real deployments need a durable queue shared between API and worker.

## What Changes

- Add `internal/jobs`: task types and versioned, validated payloads shared by producer and consumer.
- Add an asynq adapter (`internal/platform/queue`): producer client and worker mux; the asynq task id equals the job id, making enqueueing idempotent.
- JobService records every job operationally (queued/running/done/failed) so the UI reflects queue state; publish failures mark the record failed.
- The worker runs asynq with configured concurrency when `APP_REDIS_ADDR` is set, plus an optional periodic report scheduler (`APP_REPORT_CRON`); without Redis it keeps polling for development, and the UI "Process one" action is a no-op.
- Malformed payloads fail with `asynq.SkipRetry`; exhausted retries land in asynq's archive (dead-letter).
- Redis end-to-end integration test with testcontainers; compose wires api and worker to Redis with health gating.

## Capabilities

### New Capabilities

- `background-jobs-queue`

## Impact

- JOB-001 execution moves from in-process to the queue whenever Redis is configured; UI behavior for "Process one" changes accordingly (documented).
