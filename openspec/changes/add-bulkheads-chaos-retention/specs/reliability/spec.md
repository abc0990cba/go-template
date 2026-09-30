## Purpose

The reliability capability keeps one slow or broken dependency from consuming the process (breakers, bulkheads), keeps stored data bounded (retention sweeps), and makes failure behaviour rehearsed rather than assumed (chaos lab).

## ADDED Requirements

### Requirement: Bulkheads Bound Concurrency

Each named outbound dependency (enrichment HTTP, webhook HTTP) shall admit at most `APP_BULKHEAD_LIMIT` concurrent calls; excess callers shall fail within `APP_BULKHEAD_ACQUIRE_TIMEOUT` with a backpressure error that does not count as a breaker failure.

#### Scenario: Slow endpoint, fast rejection

- **WHEN** a webhook endpoint answers in 30s while the bulkhead limit is 16
- **THEN** in-flight pins at 16, `app_bulkhead_rejected_total` climbs, and `app_cb_state{webhooks.http}` stays closed (0)

### Requirement: Retention Sweeps Run On A Schedule

The worker shall prune expired GDPR export bundles, inbox rows older than `APP_RETENTION_PROCESSED_EVENTS`, and expired idempotency keys, and shall export `app_prune_deleted_total{domain}`.

#### Scenario: Inbox forgotten, sequence guard kept

- **WHEN** a processed event row is swept and the same event is redelivered
- **THEN** application falls through to the per-source sequence guard (stale), never re-applies

### Requirement: Fault Injection Is Rehearsable

A compose overlay shall proxy postgres/redis through Toxiproxy with scripts to inject latency or cuts, documented as game-day scenarios with hypotheses and expected (observable) outcomes.

#### Scenario: Postgres latency

- **WHEN** 2s latency is injected on every Postgres round-trip
- **THEN** pg pool saturation is visible on `app_pgpool_*`, the latency SLO burns, and no breaker trips
