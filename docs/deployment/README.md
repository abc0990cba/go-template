# Deployment

One image, five ways to run it. Every path below uses the same distroless
image (SPA embedded, `api` + `worker` binaries, `HEALTHCHECK` wired to
`/health/live`) — what changes is the orchestrator around it and how much
of the production shape you want to carry.

Start here to pick a path; the sub-pages are the references.

## Which path do I pick?

| # | Path | Command | What it is |
|---|------|---------|------------|
| a | **Hybrid local dev** | `docker compose up -d postgres redis clickhouse` + `make dev` | Infra in containers; api (air hot-reload), worker, and Vite on the host. Fastest edit→reload loop. |
| b | **Full docker-compose** | `make docker-up` | The whole stack in containers, SPA embedded, demo data seeded. One command, nothing on the host but Docker. |
| c | **Single VPS** (recommended self-contained production path) | `make vps-setup`, then `make vps-deploy` | Ansible provisions one Ubuntu VPS: Nomad orchestrates, Caddy terminates TLS, Postgres/Redis/registry/CI/monitoring/backups included. |
| d | **Kubernetes** | `helm upgrade --install go-template deploy/helm/go-template …` | The Helm chart with HPA, PDB, probes, optional Argo Rollouts canary and Argo CD GitOps. Bring your own databases. |
| e | **kind cluster demo** | `make kind-up` / `make kind-release` / `make kind-down` | A disposable local Kubernetes cluster running the app as a metric-gated canary — a delivery-mechanics playground, not a hosting story. |

### Comparison

| Dimension | a. Hybrid dev | b. docker-compose | c. Single VPS (Ansible+Nomad+Caddy) | d. Kubernetes (Helm) | e. kind demo |
|---|---|---|---|---|---|
| Effort to first run | Minutes (Docker + Go + Node) | One command | ~20 min incl. DNS (`vps/QUICKSTART.md`) | Cluster + Helm + managed Postgres/Redis | `make kind-up` (~5 min bootstrap) |
| Prerequisites | Docker, Go, Node | Docker only | One VPS + DNS A records + Ansible (`make vps-setup` installs nothing else by hand) | A cluster, Helm, external databases | docker, kind, kubectl, helm |
| TLS | None (http://localhost) | None | Yes — Caddy + Let's Encrypt (staging CA by default; `make vps-acme-production`) | Ingress (off by default), TLS-ready | None |
| Observability | Optional `--profile observability` | Optional `--profile observability` | Shipped: Prometheus + Grafana + alerts (plus Loki and Uptime Kuma toggles) | Point `config.otlpEndpoint` at your collector; scrape annotations included | Prometheus + Argo Rollouts dashboard, nothing more |
| CI/CD story | None (you are the CD) | None | Woodpecker on the VPS: tag → build → scan → deploy → smoke (`.woodpecker/`) | GitHub Actions `release.yml` → GHCR image + chart; optional Argo CD pulls | `make kind-release` simulates a release |
| HA | None | None | Single node; canary + auto-revert on deploy; nightly backups | HPA 2→10 api pods, PDB, rolling/canary updates | None (single control-plane node) |
| Canary / rollback | n/a | n/a | Nomad `update { canary, auto_revert }` gated on `/health/ready`; `make vps-rollback` | Argo Rollouts 20→50→100% gated on Prometheus SLO queries; auto-abort | Same Rollout machinery, watched live |
| Best for | Everyday feature work | Demos, integration checks, "does the whole thing boot" | Shipping the template for real without running a cluster team | Production at scale, progressive delivery, GitOps | Learning/rehearsing the K8s delivery path |

Deeper orchestrator comparison (K8s vs Nomad vs Swarm, and why Swarm is
documented but not shipped): [orchestrators.md](orchestrators.md).

## Environment matrix

What actually differs between environments — every knob below is a real
config value (see `../reference/configuration.md` for the full list):

| Concern | Local dev | Demo (compose / kind / nomad-dev) | Staging | Production |
|---|---|---|---|---|
| Secrets | Dev values are fine (`APP_JWT_SECRET` in the gitignored override) | Demo seed creds (`demo@go-template.local`) | Real secrets: `secrets.existingSecret` (K8s) or the Ansible vault | Same as staging; `APP_DEMO_*` must stay unset |
| `APP_COOKIE_SECURE` | `0` (plain http behind a local port) | `0` | `1` | `1` (chart and job defaults) |
| TLS | None | None | Let's Encrypt staging CA (VPS) or ingress TLS | Let's Encrypt production (`make vps-acme-production`) or ingress TLS |
| Replicas | 1 api + 1 worker | 1 (kind demo uses 3 to make the canary visible) | 2+ api (HPA floor) | HPA 2→10 @ 70% CPU; 1 worker (scale by partition) |
| Observability | Off unless `--profile observability` | Kind demo: Prometheus only | On: OTLP endpoint set, Prometheus scraping | On; alerting + SLO rules active (`../operations/observability.md`) |
| Demo data | `APP_DEMO_SEED=1` on | On | Off | Off — `APP_DEMO_EMAIL/PASSWORD/SEED` unset |
| Release path | `make dev` rebuilds on save | `make kind-release` / `nomad job run` | Tag push → Woodpecker or GHCR + Argo CD | Same, with canary gates + rollback (see `ci-cd.md`) |

## The sub-pages

| Page | Contents |
|---|---|
| [docker.md](docker.md) | The 3-stage `Dockerfile`, every compose service + profile + overlay, ports, day-to-day operations |
| [kubernetes.md](kubernetes.md) | Helm chart anatomy, Argo Rollouts canary, Argo CD handoff, the kind demo, production notes |
| [nomad.md](nomad.md) | Both Nomad job files, the variable set, canary/auto-revert, how Ansible and Woodpecker consume the same spec |
| [ci-cd.md](ci-cd.md) | GitHub Actions (CI, release, DAST, release-drafter) and the Woodpecker VPS pipeline; release checklist |
| [vps/QUICKSTART.md](vps/QUICKSTART.md) | Zero → HTTPS on one VPS in ~20 minutes |
| [vps/deployment.md](vps/deployment.md) | Full VPS reference: architecture, security model, sizing, knobs, CI setup, backups |
| [vps/runbook.md](vps/runbook.md) | Day-2 VPS operations: deploys, rollback, logs, restore |
| [vps/LOCAL-REHEARSAL.md](vps/LOCAL-REHEARSAL.md) | Rehearse the entire VPS deploy on an OrbStack VM (`make vps-local`) |
| [gitops.md](gitops.md) | Argo CD: the cluster pulls the chart from GHCR; promotion is a git commit |
| [orchestrators.md](orchestrators.md) | K8s vs Nomad vs Swarm — the decision trail |

## The release pipeline

Both CI paths start from the same event — pushing a `v*` tag — and produce
the same immutable artifact: a built-once image that deploys unchanged.

- **GitHub Actions** ([ci-cd.md](ci-cd.md)): multi-arch image → GHCR, Trivy
  scan, Syft SBOM, cosign keyless signature, SLSA provenance, packaged Helm
  chart to `oci://ghcr.io/<org>/charts`. The portable, fallback path.
- **Woodpecker on your VPS** ([ci-cd.md](ci-cd.md), the primary path when
  self-hosting): build → Trivy gate → local registry → `nomad job run` with
  a blocking health gate → smoke check.

## Where to go next

- First run of the stack: `../getting-started/quickstart.md`
- All `make` targets mentioned here: `../reference/make-targets.md`
- Dashboards, traces, alerts: `../operations/observability.md`
- Resilience knobs and the chaos lab: `../operations/reliability.md`,
  `../operations/chaos.md`
- SLO budgets the canary gate reuses: `../operations/slo.md`
- The architecture view of all of this:
  `../architecture/13-deployment-and-operations.md`; the progressive
  delivery decision is
  [ADR 0024](../architecture/decisions/0024-progressive-delivery.md).
