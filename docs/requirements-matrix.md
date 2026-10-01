# Requirements Matrix

| ID | Feature | Status | Interface | Notes |
|---|---|---|---|---|
| AUTH-001 | Register with email/password | Implemented | UI + API | Uses Argon2id |
| AUTH-002 | Login/logout | Implemented | UI + API | Cookie session |
| AUTH-003 | Link Google/GitHub/Facebook/Twitter provider | Implemented | UI + API | OAuth2 authorization-code linking; Twitter via PKCE |
| USER-001 | CRUD users | Implemented | UI + API create/list | Update/delete UI implemented |
| USER-002 | Offset/cursor/keyset pagination | Implemented | UI + API | SQL (sqlc) + in-memory, verified by store contract suite |
| JOB-001 | Enqueue enrichment job | Implemented | UI | In-memory queue |
| JOB-002 | Bounded concurrent external requests | Implemented | Worker service | randomuser.me client |
| REPORT-001 | Generate user report | Implemented | UI | Synchronous scaffold |
| ANALYTICS-001 | Analytics dashboard | Implemented | UI + API | In-memory summary, ClickHouse schema scaffolded |
| REL-001 | Graceful shutdown | Implemented | API + worker | Context-based |
| API-001 | OpenAPI contract | Implemented | `/openapi.yaml`, `/docs` | Scalar page |
| MON-001 | Dashboards, alert rules, SLO burn alerts as code | Implemented | Grafana (:3000), Prometheus (:9090/alerts), Alertmanager (:9093) | ADR 0022; provisioned by the observability profile |
| MON-002 | Synthetic outside-in health probe | Implemented | blackbox-exporter (:9115) | `LiveProbeFailed` alert; `docs/observability.md` |
| OBS-001 | Log↔trace correlation + configurable sampling | Implemented | log lines + Jaeger | `observ.TraceHandler`, `request.id` span attr, `APP_OTLP_SAMPLER` (ADR 0023) |
| OBS-002 | Spans for Redis, ClickHouse, outbound HTTP | Implemented | Jaeger | otelhttp / redisotel / driver WithSpan |
| OBS-003 | pgxpool saturation metrics | Implemented | `/metrics` `app_pgpool_*` | hand-written `prometheus.Collector` |
| OBS-004 | Continuous profiling | Implemented | Pyroscope (:4040), `APP_PYROSCOPE_URL` | off by default; flame graphs in Grafana |
| REL-002 | Bulkheads per outbound dependency | Implemented | `app_bulkhead_*` metrics | ADR 0025; enrichment + webhook fan-out |
| REL-003 | Retention sweeps (export ZIPs, inbox, idempotency) | Implemented | worker loops, `app_prune_deleted_total` | ClickHouse 90d TTL migration; inbox window `APP_RETENTION_PROCESSED_EVENTS` |
| REL-004 | Chaos fault injection | Implemented | `make chaos-*`, docs/chaos.md | Toxiproxy overlay; game-day scenarios |
| CD-001 | Canary with metric-gated auto-rollback | Implemented | `rollout.enabled=true` (Argo Rollouts) | ADR 0024; `make kind-up` local rehearsal |
| CD-002 | GitOps handoff | Implemented | `deploy/gitops/application.yaml` | Argo CD, automated sync + selfHeal |
| CD-003 | Nomad alternative | Implemented | `deploy/nomad/`, `make nomad-*` | native canary + auto_revert; docs/orchestrators.md |
| CD-004 | Release velocity gates | Implemented | CI codegen job, SLSA attestation, release-drafter | drift gate now real; provenance verifiable via `gh attestation verify` |
