# Tasks

## 1. Platform

- [x] 1.1 Circuit breaker (`internal/reliability/circuitbreaker.go`): consecutive-failure trip, half-open probe, registry, state hooks; race-tested
- [x] 1.2 Config: Kafka/NATS, breaker, outbox, rate-limit, idempotency fields with strict validation
- [x] 1.3 Metrics: `app_cb_state`, `app_enrichment_events_total`, outbox/dead-letter gauges, rate-limit and replay counters
- [x] 1.4 Breakers wrapped around external enrichment HTTP, webhook delivery, broker publishers

## 2. Persistence

- [x] 2.1 Migration `000013_streams`: users enrichment columns (+unique external_ref), `processed_events`, `outbox`, `dead_letters`, `idempotency_keys`
- [x] 2.2 sqlc queries (`streams.sql`, users columns); `make generate`
- [x] 2.3 Ports: `StreamsRepo`, `IdempotencyRepo` with compile-time assertions
- [x] 2.4 PostgreSQL implementation: one-tx apply (row lock → inbox dedupe → seq guard → JSONB merge → outbox), leased outbox claims, dead letters, idempotency
- [x] 2.5 MemoryStore twins + contract suite extension (testcontainers)

## 3. Streams Domain

- [x] 3.1 Event envelope decode/validation with `PermanentError` classification
- [x] 3.2 `StreamsService` apply path with unknown-user retries, dead-letter parking, replay
- [x] 3.3 Outbox relay pass with leased claims, breaker-wrapped publish, max-attempt dead-lettering
- [x] 3.4 Concurrency test: per-source ordered streams, concurrent sources, duplicate deliveries — zero lost updates, exactly-once outbox

## 4. Adapters And Processes

- [x] 4.1 Kafka adapter (franz-go): manual commits after tx, ordered batch processing, breaker-guarded produce, header trace propagation
- [x] 4.2 NATS adapter (JetStream): durable pull consumer, ack/nak/terminal ack, `Nats-Msg-Id` dedupe, breaker-guarded publish
- [x] 4.3 `bootstrap.Brokers` optional-infra wiring; worker supervision with backoff, graceful drain; relay + idempotency loops
- [x] 4.4 docker-compose.brokers.yml (Redpanda + NATS) and api readiness probes

## 5. HTTP And SPA

- [x] 5.1 Streams admin API: outbox list, dead-letter list, audit-logged replay; `streams` RBAC resource + role policies
- [x] 5.2 Distributed rate limiting (`platform/ratelimit`): Redis sliding window + memory fallback, fail-open; `/api` scope + tighter auth scope
- [x] 5.3 Idempotency middleware: reservation, replay, key-reuse 422, in-flight 409; wired on POST /api/tickets
- [x] 5.4 User enrichment in API responses; SPA user page "Profile (Kafka)" / "Contact (NATS)" cards; Streams admin page; nav + i18n

## 6. Tools And Verification

- [x] 6.1 `cmd/devtools`: kafka-profile, nats-contact, consume-user-events, demo, demo-dlq, dlq-list/replay, demo-breaker, flaky-server, hammer
- [x] 6.2 `make demo-up/demo-*` targets and `scripts/demo-idempotency.sh`; demo seed carries `external_ref`s
- [x] 6.3 Integration tests: streams contract on memory+Postgres; broker E2E (Kafka + NATS containers): merge, exactly-once under redelivery, dead letters, relay read-back
- [x] 6.4 Docs: `docs/enrichment-streams.md`, `.env.example`, README, openspec change
