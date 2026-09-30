## Why

Three deferred items from earlier ADRs landed together: ADR 0016 named bulkheads as the next resilience step; docs 03/06 listed the export ZIP / processed_events prunes as "exposed, not scheduled"; ADR 0017 left ClickHouse TTL as adopter policy. Failure behaviour also needed rehearsal tooling rather than hope (docs/chaos.md).

## What Changes

- **Bulkheads** (ADR 0025): `reliability.Bulkheads` registry — per-dependency concurrency caps with bounded slot wait; `ErrBulkheadFull` is backpressure and never counts as a breaker failure (bulkhead outermost, breaker innermost). Applied to `enrichment.http` and `webhooks.http`. Metrics: `app_bulkhead_in_flight{name}`, `app_bulkhead_rejected_total{name}`. Knobs: `APP_BULKHEAD_LIMIT` (16), `APP_BULKHEAD_ACQUIRE_TIMEOUT` (1s).
- **Chaos lab**: `docker-compose.chaos.yml` (Toxiproxy between app and postgres/redis), `scripts/chaos.sh` + `make chaos-*` targets, `docs/chaos.md` game-day scenarios (pool-as-fuse, graceful-degradation ladder, breaker vs bulkhead choreography).
- **Retention wired** (closes the deferred items): worker loops prune expired export ZIPs hourly (`ExportService.Prune`), sweep `processed_events` older than `APP_RETENTION_PROCESSED_EVENTS` (30d default, new `StreamsRepo.PruneProcessedEvents` implemented by both store twins + contract test), and `app_prune_deleted_total{domain}` counts swept rows. ClickHouse TTL migration `002_events_ttl.sql` (90d).
- **Env docs**: `.env.example` gains bulkhead + retention knobs.

## Capabilities

### Modified Capabilities

- `reliability` (bulkhead pattern, retention sweeps, chaos rehearsal)

## Impact

- No schema migration in Postgres (prune uses the existing inbox table); ClickHouse gains a TTL-altering migration. New unit tests (bulkhead) and a contract subtest (prune semantics vs the sequence guard).
