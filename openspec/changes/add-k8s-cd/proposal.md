## Why

The app runs on docker compose only — the largest remaining gap between "runs" and "deployable". Production readiness means a chart with sane scaling and disruption budgets, and a release pipeline that scans, bills of materials, and signs what it publishes.

## What Changes

- **Helm chart** `deploy/helm/go-template`: api + worker Deployments (probes wired to the existing `/health/startup|live|ready` endpoints; zero-downtime rolling for the api), Service, optional Ingress, HPA (CPU 70%, 2→10), PDB, ServiceAccount, securityContext (nonroot distroless, read-only rootfs), external-data defaults (Postgres/Redis/ClickHouse/OTLP via values), and a secrets pattern mounting `/etc/go-template-secrets` wired through `<VAR>_FILE`. Compose parity documented; `make helm-lint` / `helm-template` targets.
- **Release workflow** `.github/workflows/release.yml` (tag-triggered): multi-arch build → GHCR push → Trivy (fail on CRITICAL/HIGH unfixed, SARIF to code scanning) → Syft SPDX SBOM → cosign keyless signing; chart lint + OCI push to GHCR.
- **CI hardening**: all actions SHA-pinned in both workflows; golangci-lint version pinned to v2.x matching `.golangci.yml`'s config format.

## Capabilities

### New Capabilities

- `deployment`

## Impact

- No application code changes; the chart consumes the existing health/metrics surface.
- Release assumes `GITHUB_TOKEN` package write + `id-token` for keyless signing.
