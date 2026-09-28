# streams

## Purpose

Streams define how user data merges from independent external event sources without lost updates, duplicates, or lost events — and how operators inspect and repair the flow.

## ADDED Requirements

### Requirement: Exactly-Once Enrichment Application

The system shall apply each enrichment event at most once per `(source, event_id)`, regardless of broker redelivery, via an inbox table written in the same transaction as the merge.

#### Scenario: Broker redelivers an applied event

- **WHEN** the same Kafka or NATS event arrives twice
- **THEN** the second delivery is recorded as deduped and the user row is unchanged

#### Scenario: Out-of-order sequence numbers

- **WHEN** an event arrives whose per-source `seq` is not newer than the applied one
- **THEN** it is recorded as stale, the user row keeps the newer payload, and the event is not reprocessed

### Requirement: Field Ownership

Each source shall merge into exactly one user column (Kafka → `enrich_profile`, NATS → `enrich_contact`) so concurrent events from different brokers serialize on the row lock without overwriting each other.

#### Scenario: Concurrent events from both brokers

- **WHEN** profile and contact events for the same user apply concurrently
- **THEN** both payloads land fully and no field from either source is lost

### Requirement: Atomic State And Event

The system shall write the `user.enriched` outbox row in the same transaction as the merge, so a crash can never leave the row changed without its event or vice versa.

#### Scenario: Applied event

- **WHEN** an enrichment event applies successfully
- **THEN** exactly one outbox row exists for it; deduped and stale applications write none

### Requirement: Broker Resilience

Consumers shall commit/ack only after the apply transaction commits, retry transient failures with backoff (backpressuring the partition or nacking with delay), and park terminal failures — malformed payloads, unknown users — in the dead-letter table before acknowledging.

#### Scenario: Database unavailable

- **WHEN** the store cannot commit
- **THEN** offsets are not advanced and messages redeliver once the store recovers

#### Scenario: Poison message

- **WHEN** a payload cannot be decoded or matches no user after brief retries
- **THEN** it is parked in `dead_letters` with its reason and the message is acknowledged

### Requirement: Transactional Outbox Relay

The worker shall claim outbox batches with row-level leases (`FOR UPDATE SKIP LOCKED`), publish each event through a circuit breaker with server-side deduplication (message key / `Nats-Msg-Id`), and mark published rows processed; exhausted attempts land in dead letters.

#### Scenario: Crash between publish and mark

- **WHEN** the worker dies after publishing but before marking processed
- **THEN** the broker's deduplication prevents a second copy when the lease expires and the relay retries

### Requirement: Circuit Breakers

Outbound calls (external enrichment HTTP, webhook delivery, broker publishes) shall run behind named circuit breakers that trip open after consecutive failures, fail fast while open, probe with one half-open request, and export state and transition metrics.

#### Scenario: Dependency outage

- **WHEN** an upstream fails repeatedly
- **THEN** the breaker opens, calls fail fast with a typed error, `app_cb_state{name}` reads open, and a successful probe closes it after the cool-down

### Requirement: Dead-Letter Inspection And Replay

The system shall expose pending dead letters and the outbox through admin API endpoints gated by the `streams` RBAC resource, and shall replay a parked message through its live handler path with an audit record.

#### Scenario: Replay after the user arrives

- **WHEN** an `unknown_user` letter is replayed once its user exists
- **THEN** the event applies exactly once and the letter is stamped replayed; a second replay is rejected

### Requirement: Distributed Rate Limiting

The `/api` group shall enforce a per-client-IP budget shared across api processes through a Redis sliding window, with per-process token buckets as the fallback and fail-open behavior when the limiter is unavailable.

#### Scenario: Hammered endpoint

- **WHEN** requests exceed the api budget
- **THEN** excess requests receive 429 with `Retry-After` and `app_rate_limit_rejected_total{scope}` counts them

### Requirement: API Idempotency Keys

POST routes shall honor an `Idempotency-Key` header: the first request reserves the key atomically and stores its response; identical retries replay the stored response; key reuse with a different body is rejected; concurrent duplicates receive 409 until the first completes.

#### Scenario: Client retries a filed ticket

- **WHEN** the same POST arrives twice with one key and one body
- **THEN** exactly one ticket is created and the retry carries the original response with `Idempotency-Replayed: true`

### Requirement: Local Testing Tools

The template shall ship tools that exercise every stream and resilience feature locally: publish events into Kafka and NATS, read relayed events, drive the breaker and rate-limit demos, and replay dead letters — runnable with `make demo-*` against `docker compose -f docker-compose.yml -f docker-compose.brokers.yml up`.

#### Scenario: Enrichment demo

- **WHEN** `make demo-enrichment` runs against the brokers stack
- **THEN** both events land on the targeted demo user and the merged state is visible via the API and the SPA user page
