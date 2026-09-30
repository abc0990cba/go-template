## 1. Bulkhead
- [x] 1.1 `reliability/bulkhead.go` (semaphore, bounded wait, nil-safe registry, observers) + race-tested
- [x] 1.2 Config knobs; observ metrics; wiring in both mains and the service container
- [x] 1.3 Applied around `enrichment.http` and `webhooks.http` (bulkhead outermost)

## 2. Retention
- [x] 2.1 `PruneProcessedEvents` query + sqlc regen + postgres/memory implementations
- [x] 2.2 Worker loops: export ZIP prune, processed_events sweep, prune metric
- [x] 2.3 ClickHouse TTL migration; contract subtest proves dedupe-vs-sequence semantics

## 3. Chaos
- [x] 3.1 `docker-compose.chaos.yml` (Toxiproxy + aliases + rewired endpoints)
- [x] 3.2 `scripts/chaos.sh` + make chaos-* targets; `docs/chaos.md` scenarios
