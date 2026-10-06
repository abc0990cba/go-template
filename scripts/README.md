# scripts/ — developer & demo shell scripts

Standalone shell scripts behind the `make` targets; each is runnable directly too. See [../docs/reference/make-targets.md](../docs/reference/make-targets.md) for the wrapping targets and flags.

| Script | Purpose |
| --- | --- |
| `dev.sh` | One-terminal dev loop (`make dev`): runs the Go API with air hot-reload, the Vite dev server, and a Postgres container; tears everything down on Ctrl-C. |
| `chaos.sh` | Toxiproxy control for the chaos lab (`make chaos-*`): inject latency, drop Postgres/Redis, restore. Scenarios documented in [../docs/operations/chaos.md](../docs/operations/chaos.md); needs the chaos compose profile. |
| `kind-demo.sh` | Local progressive-delivery walkthrough (ADR 0024): `up` builds a kind cluster with Prometheus + Argo Rollouts running the API as an SLO-gated canary. |
| `demo-idempotency.sh` | POSTs the same ticket twice with one `Idempotency-Key` against the demo stack and shows the replayed response. |
| `regression-walkthrough.sh` | 33-check live API gauntlet against a seeded stack: `BASE=http://localhost:18080 ./scripts/regression-walkthrough.sh`. |
