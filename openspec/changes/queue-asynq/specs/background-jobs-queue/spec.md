## Purpose

The queue capability defines distributed background execution: durable handoff between API and worker, idempotent enqueueing, bounded retries, and honest operational records.

## ADDED Requirements

### Requirement: Distributed Execution

When `APP_REDIS_ADDR` is configured, jobs enqueued through the API shall be executed by the separate worker process via asynq, and their operational records shall progress queued → running → done/failed.

#### Scenario: Report job through Redis

- **WHEN** the API enqueues `user.report_generate` against a running worker
- **THEN** the worker executes the task and the job record ends `done` with a report stored

### Requirement: Idempotent Enqueueing

The queue shall reject a duplicate task id instead of creating a second job.

#### Scenario: Producer retry

- **WHEN** the same job id is published twice
- **THEN** the second publish fails with `ErrTaskIDConflict`

### Requirement: Bounded Retries And Dead-Letter

Tasks shall retry at most five times with backoff; malformed payloads shall not be retried.

#### Scenario: Corrupt payload

- **WHEN** a task carries an undecodable payload
- **THEN** the handler returns a non-retryable error and the task moves to the archive
