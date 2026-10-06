# Kubernetes / Helm path

The chart at `deploy/helm/go-template` is the production-shaped Kubernetes
deployment: two workloads (api + worker), autoscaling, disruption budget,
hardened pods, and an optional metric-gated canary. This page covers the
chart anatomy, the canary, the kind demo that rehearses it all locally, and
production notes. For K8s-vs-Nomad context see
[orchestrators.md](orchestrators.md); for the GitOps handoff see
[gitops.md](gitops.md).

## Install

```bash
make helm-lint              # chart lint (also runs in CI's release pipeline)
helm template go-template deploy/helm/go-template   # render locally, no cluster
helm upgrade --install go-template deploy/helm/go-template \
  --set image.repository=ghcr.io/your-org/go-template \
  --set image.tag=1.0.0 \
  --set-string config.databaseURL="postgres://app:***@db:5432/app?sslmode=require" \
  --set config.redisAddr="redis-master.data.svc:6379" \
  --set config.otlpEndpoint="otel-collector.observability:4318" \
  --set config.otlpSampler="0.1"
```

`image.tag` defaults to `.Chart.AppVersion`; the release pipeline keeps the
chart's `appVersion` in lockstep with the image it publishes (see
[ci-cd.md](ci-cd.md)).

## Chart anatomy

Chart version `0.1.0`, `appVersion: 1.0.0` (`Chart.yaml`). Templates:

| Template | What it renders |
|---|---|
| `deployment-api.yaml` | api Deployment — **only when `rollout.enabled=false`**. Rolling update with `maxUnavailable: 0`, `maxSurge: 1` (zero-downtime). |
| `deployment-worker.yaml` | worker Deployment (`worker.enabled`, default on). Prometheus scrape annotations on `worker.metricsAddr` (default `:9091`), exposed as a `metrics` container port and `APP_METRICS_ADDR`. |
| `hpa.yaml` | HorizontalPodAutoscaler (autoscaling/v2): CPU 70%, **2 → 10 replicas**. `scaleTargetRef.kind` follows the active workload kind — Deployment or Rollout (Rollouts implements the scale subresource). |
| `pdb.yaml` | PodDisruptionBudget for the api, `minAvailable: 1`. |
| `rollout.yaml` | Argo Rollouts canary — replaces the api Deployment when `rollout.enabled=true` (below). |
| `analysis.yaml` | The `AnalysisTemplate` gating that canary (below). |
| `secret.yaml` | Renders `secrets.data` as a Secret — skipped when `secrets.existingSecret` is set. |
| `ingress.yaml` | Optional Ingress (off by default), `className: nginx`, TLS-ready (`ingress.tls`). |
| `service-api.yaml` / `service-api-canary.yaml` | Stable + canary Services (the canary one only matters under Rollouts). |
| `serviceaccount.yaml`, `NOTES.txt` | SA + post-install hints. |

### The shared pod spec

The api pod template lives in one Helm helper (`go-template.api.podSpec` in
`_helpers.tpl`) and is included **verbatim by both the Deployment and the
Rollout** — the two shapes cannot drift.

- **Probes** wired to the app's three endpoints: `startupProbe` →
  `/health/startup` (every 2s, up to 30 failures), `livenessProbe` →
  `/health/live`, `readinessProbe` → `/health/ready` (both every
  `probe.periodSeconds`, default 10).
- **securityContext** (defaults in `values.yaml`): `runAsNonRoot: true`,
  `runAsUser: 65532`, `readOnlyRootFilesystem: true`,
  `allowPrivilegeEscalation: false`, capabilities `drop: [ALL]` — matching
  the distroless `nonroot` image.
- **Resources**: api requests `100m/128Mi`, limits `1 CPU/512Mi`; worker
  `50m/64Mi` → `500m/256Mi`.
- **Env**: `go-template.env` builds the common `APP_*` list
  (`APP_HTTP_ADDR=:8080`, `APP_DATABASE_URL`, `APP_REDIS_ADDR`,
  `APP_LOG_FORMAT`, `APP_COOKIE_SECURE`, optional `APP_CLICKHOUSE_ADDR`,
  `APP_OTLP_ENDPOINT`, `APP_OTLP_SAMPLER`), then `api.env` /
  `worker.env` / `config.extraEnv` layer on top.

### Secrets: `stringData` or `existingSecret`, consumed via `_FILE`

Two ways to supply secrets, both wired through the app's native
`<VAR>_FILE` docker-secrets pattern:

1. **Inline (`secrets.data`)** — a map rendered into a Secret
   (`stringData`). Keys ending in `_file` (e.g.
   `APP_DATABASE_URL_FILE: postgres://…`) are mounted as files under
   `/etc/go-template-secrets` and the matching `<VAR>_FILE` env var points
   there. Any other key becomes a `secretKeyRef` env var.
2. **Bring your own (`secrets.existingSecret`)** — name a Secret you manage
   (sealed-secrets, external-secrets, terraform…). The chart skips rendering
   its own and uses the same mount/env wiring against yours. Prefer this in
   real deployments.

### No bundled databases

The chart intentionally ships **no Postgres, Redis, or ClickHouse** — point
`config.databaseURL` / `config.redisAddr` / `config.clickhouseAddr` at
managed services or your own StatefulSets (`config.clickhouseAddr` stays
empty to disable ClickHouse entirely).

Sizing rule of thumb: with `APP_PG_MAX_CONNS=20` per pod (the `api.env`
default), keep `maxReplicas × 20 ≤ Postgres max_connections − overhead`.
The HPA ceiling is the knob to tune first, not the pool.

## Progressive delivery: the Argo Rollouts canary

Enable with `--set rollout.enabled=true`. The api Deployment is replaced by
an `argoproj.io/v1alpha1` Rollout (ADR 0024,
`../architecture/decisions/0024-progressive-delivery.md`).

Steps (tunable via `rollout.steps`):

1. `setWeight: 20` → pause 60s → **analysis**
2. `setWeight: 50` → pause 60s → **analysis**
3. `setWeight: 100`

Without traffic routing, weights control the **replica split** between
stable and canary (via the `rollouts-pod-template-hash` label Rollouts
manages on the two Services). With ingress-nginx installed, shift **real
traffic percentages** instead:

```bash
--set rollout.trafficRouting.enabled=true --set ingress.enabled=true
```

### The gate (`analysis.yaml`)

Each analysis runs `rollout.analysis.count` (default 3) checks every 30s
against Prometheus (`rollout.analysis.prometheusAddress`, default
`http://prometheus.observability:9090`) and aborts back to stable on the
**first breach** (`failureLimit: 0`):

| Metric | Query | Threshold |
|---|---|---|
| availability | `sum(rate(app_http_requests_total{status!~"5.."}[2m])) / sum(rate(app_http_requests_total[2m]))` | ≥ `0.99` |
| latency p95 | `histogram_quantile(0.95, sum(rate(app_http_request_duration_seconds_bucket[2m])) by (le))` | ≤ `0.5` s |

These are the *same* thresholds the SLO alerting layer uses
(`deploy/prometheus/rules/slo.yml`, `../operations/slo.md`) — the rollback
trigger is "users are having a bad time", not "the process started". Both
success conditions treat no-data (`isNaN`) as success, so a quiet canary
doesn't abort for having no traffic to measure. The queries filter per
ReplicaSet via the `rollouts-pod-template-hash` pod label when Prometheus
pod auto-discovery provides it; without pod labels they measure the whole
fleet, which is the right signal for replica-split canaries anyway.

A pause step with **no duration** waits for manual promotion
(`kubectl argo rollouts promote`).

Requires the `argo-rollouts` controller in the cluster. To see all of it in
minutes, use the kind demo below.

## The kind demo (`scripts/kind-demo.sh`)

A local rehearsal of the entire delivery path — cluster, Prometheus,
argo-rollouts, and the app installed **as a Rollout** — in one command per
step (`make kind-up` / `kind-release` / `kind-down` wrap the script):

```bash
make kind-up        # cluster go-template-demo + prometheus + argo-rollouts + app (~5 min)
make kind-release   # push a new revision, watch the canary gate
make kind-down      # tear it all down
```

What `up` does: creates the kind cluster, builds and loads the image
(`go-template:kind-demo`), installs Prometheus (prometheus-community chart,
with a scrape config discovering app pods on `:8080`), installs
argo-rollouts `v1.7.2`, then helms the chart with `rollout.enabled=true`,
`api.hpa.enabled=false`, `api.replicaCount=3`, and **empty database/redis
URLs — the app runs on its in-memory store**. This demo is about delivery
mechanics, not data.

Watch and interact:

```bash
kubectl -n go-template argo rollouts get rollout go-template-api --watch
kubectl -n go-template port-forward svc/go-template-api 8080:80
```

`release` bumps a `KIND_DEMO_REVISION` env var → new ReplicaSet → canary:
20% → analysis → 50% → analysis → 100%. Force the **abort** path by pointing
the canary at an image that does not exist:

```bash
kubectl -n go-template argo rollouts set image go-template-api \
  api=ghcr.io/your-org/go-template:nope
```

Requirements: `docker`, `kind`, `kubectl`, `helm`. The script skips steps
already done, so it is re-runnable.

## Production notes

- **Ingress**: `ingress.enabled=true`, `className: nginx` by default,
  `ingress.hosts` + `ingress.tls` per your setup. With Rollouts +
  `trafficRouting`, the stable ingress is the trafficRouting anchor.
- **Tracing**: set `config.otlpEndpoint` (e.g.
  `otel-collector.observability:4318`) and tune sampling with
  `config.extraEnv.APP_OTLP_SAMPLER` (e.g. `"0.1"` for 10% head sampling).
  Empty disables tracing.
- **Images** come from the GHCR release pipeline:
  `ghcr.io/<org>/go-template:<semver>` — multi-arch, Trivy-scanned, Syft
  SBOM, cosign-signed, SLSA-attested ([ci-cd.md](ci-cd.md)). Add
  `imagePullSecrets` for private GHCR.
- **GitOps**: instead of `helm upgrade` by hand, let Argo CD pull the
  packaged chart from `oci://ghcr.io/<org>/charts` —
  [gitops.md](gitops.md) and `deploy/gitops/application.yaml`.
- **Maintenance mode** (the `maintenance.enabled` setting) is the pause
  button during incidents; health and metrics endpoints keep serving so the
  orchestrator stays happy.
- **Verify before applying**: `helm template go-template
  deploy/helm/go-template --set rollout.enabled=true …` shows exactly which
  workload kind and analysis objects you will get.

Operational runbooks (dashboards, traces, alerts, SLO budgets):
`../operations/observability.md`, `../operations/slo.md`.
