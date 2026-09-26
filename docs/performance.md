# Performance

The first scale target is 100k users.

## Running

```bash
make docker-up           # live stack on PostgreSQL + Redis
make seed-users          # 100k users (USERS=10000 for a smaller set)
make perf-login          # login burst, p95 < 300ms
MODE=offset   make perf-users   # p95 < 500ms
MODE=cursor   make perf-users   # p95 < 300ms
MODE=keyset   make perf-users   # p95 < 250ms
make perf-analytics      # dashboard read, p95 < 500ms
make perf-enrichment     # external fan-out job, p95 < 2s, errors < 5%
make test-performance    # health smoke: p95 < 500ms, errors < 1%
```

Scripts live in `test/performance`; `BASE_URL` overrides the target. Go benchmarks for the hot paths (Argon2id hashing) run with:

```bash
go test ./internal/service/ -bench "BenchmarkHash|BenchmarkVerify" -benchtime 3x
```

Argon2id costs roughly 20ms and ~67MB per hash on the reference laptop — login throughput is bounded by CPU, not the database; tune the memory parameter deliberately if throughput matters.

Required performance scenarios and their status live in `docs/performance-matrix.md`. Record p95, p99, and error rate per run.
