# ADR 0008 — asynq on Redis for Background Jobs

**Status:** Accepted

## Context

Background work (exports, report snapshots, webhook deliveries, enrichment
batches) needs: durability across restarts, retries with backoff, delayed
and scheduled (cron) execution, dedupe on producer retries, visibility
into what ran, and a dev mode without extra infrastructure. The template
already ships Redis for cache/rate-limits — the queue should reuse it.

## Decision

Use **asynq** (Redis-backed) behind the `service.JobQueue` port:

- Four versioned task types (`user.enrich_batch`,
  `user.report_generate`, `user.export_generate`, `webhook.deliver`) on a
  single `default` queue, each with an explicit timeout.
- **`asynq.TaskID` = domain job id** makes enqueue idempotent — producer
  retries cannot duplicate work (contract-tested via
  `ErrTaskIDConflict`).
- Every task has a `jobs` table row (`queued → running → done|failed`)
  that is the status/audit surface exposed to the admin UI and SSE.
- `asynq.NewScheduler` registers cron tasks (`APP_REPORT_CRON`); retries
  use the default exponential delay; malformed payloads are marked
  `SkipRetry`.
- Dev mode without Redis: an in-process ticker polls `ProcessOne` every
  2 s, plus `POST /api/jobs/process` for on-demand processing.

## Consequences

**Positive:** production semantics (persistence, retries, scheduling,
dedupe) with one dependency the stack already has; job status lives in
Postgres where RBAC/audit can see it; the `JobQueue` port keeps a
SQS/RabbitMQ swap contained.

**Negative:** Redis persistence config becomes operationally relevant
(AOF/RDB — adopter's call); one queue means a slow job type can delay
others (acceptable at this scale; asynq supports named queues with
priorities when needed).

## Alternatives considered

- *River/queued Postgres jobs* — attractive (one less store), but
  poll-based and reimplements dedupe/scheduling; revisit if Redis ever
  leaves the stack.
- *Kafka as a work queue* — wrong semantics for per-job retry/visibility;
  Kafka here is for streams, not tasks.
- *Celery-style separate broker* — extra operational dependency for
  capabilities Redis already provides.
