# Jobs

The target queue is asynq backed by Redis.

Current scaffold includes an in-memory development job service so the UI can be tested immediately.

Job types:

- `user.enrich_batch`
- `user.report_generate`

Rules:

- Jobs must be idempotent.
- Retries must have limits.
- Failed jobs must eventually move to dead-letter handling.
- Queue concurrency is configured, not hardcoded.
- Job payloads are versioned and validated.
