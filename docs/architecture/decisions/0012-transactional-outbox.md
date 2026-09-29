# ADR 0012 — Transactional Outbox for Broker Publishing

**Status:** Accepted

## Context

Whenever a domain write must be announced to other systems (here: the
`user.enriched` event after an enrichment merge), the naive approach —
commit Postgres, then publish to Kafka/NATS — loses events on crash
between the two; publish-then-commit produces phantoms. The dual-write
problem is structural, not an implementation bug.

## Decision

The **transactional outbox** pattern: the apply transaction inserts the
outbound event into the `outbox` table (payload + trace headers);
committing makes the event exist exactly when the data exists. A relay
loop in the worker (1 s tick, batch 100) claims rows with
`FOR UPDATE SKIP LOCKED` + a 60 s lease (multi-worker safe), publishes to
both brokers (Kafka keyed by aggregate; NATS with `Nats-Msg-Id` = outbox
id for broker-side dedupe) under breakers, marks success, or increments
attempts — at `max_attempts` the event parks in the DLQ. Processed rows
are deleted after 7 days.

## Consequences

**Positive:** no lost or phantom events, ever; trace context propagates
from the originating request into broker headers; relay failure is
visible as a metric backlog, not silent data loss; the pattern is
textbook and maps 1:1 for adopters.

**Negative:** consumers see events with ≥ 1 s latency (poll interval) —
never a problem for integration events; the outbox table needs the
7-day prune (implemented); at-least-once delivery to brokers means
downstreams must dedupe (Kafka key + `Nats-Msg-Id` provided).

## Alternatives considered

- *Change Data Capture (Debezium)* — powerful, but a whole additional
  platform (Kafka Connect + connector ops) for one event type.
- *Publish-then-commit with compensation* — phantoms are worse than
  delays.
- *Poll the domain table instead of an outbox* — hidden coupling to table
  shapes and no room for envelope/trace metadata.
