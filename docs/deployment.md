# Production Deployment

The template ships two deployment artifacts: docker compose (dev parity) and
a **Helm chart** (`deploy/helm/go-template`) for Kubernetes, plus a release
pipeline that builds, scans, attests, and signs images.

## Helm chart

```bash
make helm-lint              # chart lint (also runs in CI)
helm template go-template deploy/helm/go-template   # render locally
helm upgrade --install go-template deploy/helm/go-template \
  --set image.repository=ghcr.io/your-org/go-template \
  --set image.tag=1.0.0 \
  --set-string config.databaseURL="postgres://app:***@db:5432/app?sslmode=require" \
  --set config.redisAddr="redis-master.data.svc:6379" \
  --set config.otlpEndpoint="otel-collector.observability:4318"
```

What the chart sets up:

| Piece | Detail |
|---|---|
| api Deployment | 2+ replicas, zero-downtime rolling (`maxUnavailable: 0`), probes wired to `/health/startup|live|ready`, nonroot distroless security context (read-only rootfs, all caps dropped) |
| worker Deployment | 1+ replicas, `APP_METRICS_ADDR` for the stream/outbox/DLQ gauges, Prometheus scrape annotation |
| HPA | CPU-70% scaling 2→10 api pods (matches the k6 sizing in `docs/performance-matrix.md`) |
| PDB | `minAvailable: 1` for the api |
| Ingress | optional (off by default), TLS-ready |
| Secrets | `secrets.data` → a Secret mounted at `/etc/go-template-secrets`, wired through the `<VAR>_FILE` pattern (e.g. `APP_DATABASE_URL_FILE`) |

The chart does **not** ship Postgres/Redis/ClickHouse — point
`config.databaseURL` / `config.redisAddr` at managed services.

Sizing note: with `APP_PG_MAX_CONNS=20` per pod, keep
`maxReplicas × 20 ≤ Postgres max_connections − overhead`; the HPA ceiling is
the knob to tune first, not the pool.

## Release pipeline (`.github/workflows/release.yml`)

Tag a release (`v1.2.3`) and CI builds and publishes:

1. multi-arch image (amd64 + arm64) pushed to GHCR with semver tags,
2. **Trivy** scan failing the build on CRITICAL/HIGH with unfixed CVEs
   (SARIF uploaded to code scanning),
3. **Syft** SPDX SBOM attached to the build,
4. **cosign keyless** signature (workflow OIDC),
5. Helm chart linted, packaged, and pushed to `oci://ghcr.io/<org>/charts`.

All GitHub Actions are SHA-pinned in both workflows; the golangci-lint
version is pinned to the config format the repo uses (v2).

## Operational notes

- `APP_COOKIE_SECURE=1` is the default; TLS terminates at the ingress.
- `APP_DEMO_*` must stay unset in production.
- Maintenance mode (`maintenance.enabled` setting) is your pause button
  during incidents; health/metrics keep serving so the orchestrator is happy.
- Runbooks: see `docs/observability.md` (dashboards, traces) and
  `docs/reliability.md` (resilience knobs).
