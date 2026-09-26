# Performance Matrix

| Scenario | Script | Target | Status |
|---|---|---|---|
| Health smoke | `test/performance/smoke.js` | p95 < 500ms, errors < 1% | Implemented |
| Login burst | `test/performance/login.js` | p95 < 300ms | Implemented (run `make perf-login`) |
| Users offset pagination | `test/performance/users-list.js` `MODE=offset` | p95 < 500ms on 100k users | Implemented (run `MODE=offset make perf-users`) |
| Users cursor pagination | `test/performance/users-list.js` `MODE=cursor` | p95 < 300ms on 100k users | Implemented (run `MODE=cursor make perf-users`) |
| Users keyset pagination | `test/performance/users-list.js` `MODE=keyset` | p95 < 250ms on 100k users | Implemented (run `MODE=keyset make perf-users`) |
| Enrichment batch | `test/performance/enrichment.js` | p95 < 2s, errors < 5% | Implemented (run `make perf-enrichment`) |
| Analytics dashboard | `test/performance/analytics.js` | p95 < 400ms | Implemented (run `make perf-analytics`) |
| Hash hot path | `go test -bench` in `internal/service` | ~20ms per Argon2id hash on the reference laptop | Measured |

Recorded runs (fill in p95/p99/error rate per environment):

| Scenario | Environment | p95 | p99 | Errors |
|---|---|---|---|---|
| Users offset (n=300 sequential) | local, 20k seeded users, OrbStack PG 16 | 0.6ms | 0.7ms | 0 |
| Users cursor (n=300 sequential) | local, 20k seeded users, OrbStack PG 16 | 1.9ms | 2.0ms | 0 |
| Users keyset (n=300 sequential) | local, 20k seeded users, OrbStack PG 16 | 1.9ms | 2.0ms | 0 |
| Seed throughput | `cmd/seed`, pgx COPY | 20k users in 251ms | — | — |

Sequential probes are not load tests; run the k6 scenarios for concurrent numbers. The first measured run shows the 250–500ms thresholds have orders of magnitude of headroom.
