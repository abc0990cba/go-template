# Production Deployment

The template ships deployment artifacts for two orchestrators — a **Helm
chart** (`deploy/helm/go-template`) for Kubernetes and a **Nomad job**
(`deploy/nomad/`) — plus docker compose for dev parity, a release pipeline
that builds, scans, attests, and signs images, and a GitOps handoff that
lets the cluster pull instead of a human pushing. See
[docs/orchestrators.md](orchestrators.md) for the comparison and why
Swarm is documented, not shipped.

## Helm chart

```bash
make helm-lint              # chart lint (also runs in CI)
helm template go-template deploy/helm/go-template   # render locally
helm upgrade --install go-template deploy/helm/go-template \
  --set image.repository=ghcr.io/your-org/go-template \
  --set image.tag=1.0.0 \
  --set-string config.databaseURL="postgres://app:***@db:5432/app?sslmode=require" \
  --set config.redisAddr="redis-master.data.svc:6379" \
  --set config.otlpEndpoint="otel-collector.observability:4318" \
  --set config.otlpSampler="0.1"
```

What the chart sets up:

| Piece | Detail |
|---|---|
| api Deployment | 2+ replicas, zero-downtime rolling (`maxUnavailable: 0`), probes wired to `/health/startup|live|ready`, nonroot distroless security context (read-only rootfs, all caps dropped) |
| worker Deployment | 1+ replicas, `APP_METRICS_ADDR` for the stream/outbox/DLQ gauges, Prometheus scrape annotation |
| HPA | CPU-70% scaling 2→10 api pods (matches the k6 sizing in `docs/performance-matrix.md`); target follows the active workload kind (Deployment or Rollout) |
| PDB | `minAvailable: 1` for the api |
| Ingress | optional (off by default), TLS-ready |
| Secrets | `secrets.data` → a Secret mounted at `/etc/go-template-secrets`, wired through the `<VAR>_FILE` pattern (e.g. `APP_DATABASE_URL_FILE`) |

The chart does **not** ship Postgres/Redis/ClickHouse — point
`config.databaseURL` / `config.redisAddr` at managed services.

Sizing note: with `APP_PG_MAX_CONNS=20` per pod, keep
`maxReplicas × 20 ≤ Postgres max_connections − overhead`; the HPA ceiling is
the knob to tune first, not the pool.

## Progressive delivery (canary) — `rollout.enabled=true`

Setting `rollout.enabled` swaps the api Deployment for an **Argo Rollouts
canary**: 20% → SLO analysis → 50% → analysis → 100%, where the
`AnalysisTemplate` polls Prometheus for the *same* thresholds the alerting
layer uses (`docs/slo.md`) — availability ≥ 99%, p95 ≤ 500 ms — and the
first breach aborts back to stable automatically (ADR 0024). The pod spec
is shared between both shapes via a Helm helper, so they cannot drift.

```bash
# Local rehearsal end to end (kind cluster, ~5 min):
make kind-up        # cluster + prometheus + argo-rollouts + the app as a Rollout
make kind-release   # push a new revision, watch the canary gate (and force an abort)
make kind-down

# With ingress-nginx, shift real traffic percentages instead of replicas:
--set rollout.trafficRouting.enabled=true --set ingress.enabled=true
```

Requires `argo-rollouts` in the cluster. A pause step with no duration
waits for manual promotion (`kubectl argo rollouts promote`).

## GitOps (`deploy/gitops/`)

[`deploy/gitops/application.yaml`](../deploy/gitops/application.yaml) hands
the cluster to Argo CD: it pulls the packaged chart from `oci://ghcr.io`,
syncs automatically, and reverts manual drift (`selfHeal`). Promotion is a
git commit; rollback is `git revert`. The full flow and operating notes
live in [docs/gitops.md](gitops.md).

## Nomad alternative (`deploy/nomad/`)

The same api + worker pair as a Nomad job with native canary
(`update { canary = 1, auto_promote, auto_revert }`) gated on
`/health/ready`. Health checks cannot see SLO regressions the way the
Rollouts analysis can — that trade and the rest of the comparison are in
[docs/orchestrators.md](orchestrators.md).

```bash
make nomad-up       # single dev agent on your machine
make nomad-run      # submit the job; watch http://localhost:4646
```

## Release pipeline (`.github/workflows/release.yml`)

Tag a release (`v1.2.3`) and CI builds and publishes:

1. multi-arch image (amd64 + arm64) pushed to GHCR with semver tags,
2. **Trivy** scan failing the build on CRITICAL/HIGH with unfixed CVEs
   (SARIF uploaded to code scanning),
3. **Syft** SPDX SBOM attached to the build,
4. **cosign keyless** signature (workflow OIDC),
5. **SLSA build provenance** attestation (`gh attestation verify ... -R <repo>`),
6. Helm chart linted, packaged, and pushed to `oci://ghcr.io/<org>/charts`.

Tagged releases also get **drafted notes** grouping merged PRs by category
(`release-drafter` workflow; categories come from PR title prefixes in
`.github/release-drafter.yml`).

CI guardrails: all actions are SHA-pinned in both workflows; the
golangci-lint version is pinned to the config format the repo uses (v2);
and a **codegen drift gate** regenerates sqlc/oapi-codegen and fails the
PR on any diff, so the generated code can never silently lag its sources.

## Operational notes

- `APP_COOKIE_SECURE=1` is the default; TLS terminates at the ingress.
- `APP_DEMO_*` must stay unset in production.
- Maintenance mode (`maintenance.enabled` setting) is your pause button
  during incidents; health/metrics keep serving so the orchestrator is happy.
- Runbooks: see `docs/observability.md` (dashboards, traces, alerts),
  `docs/slo.md` (budgets), and `docs/reliability.md` (resilience knobs,
  chaos lab).
