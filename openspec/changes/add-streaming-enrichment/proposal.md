# Add Streaming Enrichment

## Why

The template enriches users only via a polling batch job. Real systems merge data from several independent producers — and doing that correctly is where production backends actually get hard: two brokers deliver at-least-once, rows are updated concurrently, and a crash between a database commit and a broker publish silently loses events. The template also lacked the resilience toolkit reviewers expect: no circuit breaker, no transactional messaging, no dead-letter story, process-local rate limiting, and no idempotency for unsafe retries.

## What Changes

- **Kafka + NATS user enrichment**: a franz-go consumer group on `user-profile.v1` and a JetStream durable pull consumer on `users.contact.v1` merge owned JSONB field groups into user rows. Exactly-once per `(source, event_id)` via an inbox table inside the apply transaction; per-source monotonic `seq` guards drop stale events; each source owns exactly one column so cross-broker concurrency serializes harmlessly on the row lock; every applied event commits a `user.enriched` outbox row atomically.
- **Transactional outbox**: relay in the worker claims leased batches (`FOR UPDATE SKIP LOCKED`), publishes through a circuit breaker to `user-events.v1` (Kafka) and `users.events.v1` (NATS with `Nats-Msg-Id` server-side dedupe), and parks exhausted events in dead letters.
- **Circuit breakers** (`internal/reliability`): stdlib-only, consecutive-failure trip, half-open probe, state gauges; wrapped around external enrichment HTTP, webhook delivery, and both broker publishers.
- **Dead-letter queue + replay console**: one `dead_letters` table, admin API (`/api/streams/dead-letters`, `…/{id}/replay`) behind a new `streams` RBAC resource, audit-logged replays, Streams page in the SPA.
- **Distributed rate limiting**: Redis sliding-window limiter for the whole `/api` group (per-process token buckets as fallback), fail-open on limiter outages.
- **API idempotency keys**: `Idempotency-Key` middleware on POST routes; reservation via `INSERT … ON CONFLICT`, response replay on identical retries, 422 on key reuse, 409 while in flight.
- **Local testing tools**: `cmd/devtools` publishes into Kafka/NATS, reads relayed events, runs the breaker and rate-limit demos; `make demo-up` + `demo-*` targets; docker-compose.brokers.yml brings up Redpanda and NATS; demo seed users carry `external_ref`s so everything works out of the box.

## Capabilities

### New Capabilities

- `streams`

## Impact

- New dependencies: `twmb/franz-go`, `nats-io/nats.go`, testcontainers kafka+nats modules.
- Migration `000013_streams` adds users enrichment columns and four tables.
- Worker runs the consumers, the relay, and the idempotency sweep; api gains broker readiness pings.
- Both rate-limit scopes and idempotency are opt-in by configuration and degrade gracefully when their backend is off — consistent with the template's optional-infrastructure rule.
