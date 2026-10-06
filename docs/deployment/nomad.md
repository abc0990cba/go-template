# Nomad path

The repo ships two Nomad job files with different jobs in life:

| File | Role |
|---|---|
| `deploy/nomad/go-template.nomad.hcl` | **Local demo** — a teaching walkthrough against a dev agent on your machine |
| `deploy/nomad/app.nomad.hcl` | **Production** — the single source of truth for app deploys, consumed unchanged by both the Ansible VPS path and the Woodpecker pipeline |

Why Nomad exists here at all, and how it compares to Kubernetes:
[orchestrators.md](orchestrators.md). The VPS that runs this job spec is
documented in [vps/QUICKSTART.md](vps/QUICKSTART.md) and
[vps/deployment.md](vps/deployment.md).

## The local demo: `go-template.nomad.hcl`

A self-contained walkthrough (ADR 0024): Nomad's `update` block gives canary
+ auto-revert natively — one canary allocation runs beside stable, health
checks must pass, and a failed deployment reverts without a human.

```bash
make nomad-up        # nomad agent -dev in the background (log: /tmp/nomad-dev.log)
make nomad-validate  # validate the job file
make nomad-run       # submit the job
open http://localhost:4646   # watch the deployment + canary placement
make nomad-stop      # stop the dev agent
```

Shape of the demo job:

- api group `count = 2` with a **static port** `8080 → 8080`; worker group
  `count = 1` with `APP_METRICS_ADDR=:9090`.
- `update { canary = 1, auto_promote = true, auto_revert = true,
  min_healthy_time = "15s" }`; the service check polls
  **`/health/ready`** every 10s.
- Image `ghcr.io/your-org/go-template:latest`; env points at the **compose
  stack on your host** via `host.docker.internal` — Postgres on
  `55432` (`postgres://app:app@host.docker.internal:55432/app`), Redis on
  `6379`, Jaeger OTLP on `4318`. Start `docker compose up -d postgres redis`
  (and the observability profile for traces) first.
- Dev-shaped on purpose: `APP_COOKIE_SECURE=0`, demo seed enabled
  (`APP_DEMO_SEED=1`, login `demo@go-template.local` /
  `demo-password-123`).

## Production: `app.nomad.hcl`

This one file is the **single source of truth** — both deploy paths submit
it unchanged and differ only in `-var` values:

- `make vps-deploy` / `make vps-app-deploy TAG=v1.2.3` (Ansible passes
  `-var`s from the inventory), and
- the Woodpecker pipeline on the VPS ([ci-cd.md](ci-cd.md)).

### Variables

| Variable | Default | Meaning |
|---|---|---|
| `app_image` | `127.0.0.1:5000/go-template` | Image repository, no tag (the VPS-local registry; switch to `ghcr.io/<org>/go-template` to consume GitHub releases) |
| `image_tag` | `v0.1.0` | Immutable tag produced by CI or the bootstrap build |
| `app_domain` | `localhost` | Public hostname served by Caddy; feeds `APP_BASE_URL` (`https://<app_domain>`) |
| `api_replicas` | `1` | api allocation count (multi-node scaling is a Phase-2 change) |
| `report_cron` | `0 3 * * *` | Nightly report schedule for the worker |
| `smtp_host` / `smtp_port` / `smtp_username` / `smtp_from` | `""` / `587` / `""` / `""` | Outbound mail |

### Design notes (all baked into the file)

- **Secrets never appear in the file.** Both groups mount the host volume
  `secrets` read-only at `/mnt/secrets`, and env points at files:
  `APP_DATABASE_URL_FILE=/mnt/secrets/database_url`,
  `APP_JWT_SECRET_FILE=/mnt/secrets/jwt_secret`,
  `APP_SMTP_PASSWORD_FILE=/mnt/secrets/smtp_password`. The app reads
  `*_FILE` natively (its docker-secrets pattern).
- **Host networking** (`network_mode = "host"`): every service binds the
  node's loopback — Postgres, Redis, and the worker metrics listener
  (`APP_METRICS_ADDR=127.0.0.1:9090`, checked via a `/metrics` service
  check every 15s). Only the api listens on `:8080`, and only Caddy exposes
  80/443 publicly (UFW enforces this).
- **Canary + auto-revert on `/health/ready`.** The api group's
  `update { max_parallel = 1, min_healthy_time = "15s",
  healthy_deadline = "5m", canary = 1, auto_promote = true,
  auto_revert = true }`: a new allocation must pass the HTTP check
  (`/health/ready`, every 10s, timeout 3s) before promotion; a failing
  deployment reverts to the previous version with no human action. The
  worker group gets the same treatment minus the canary
  (`min_healthy_time = "10s"`, `auto_revert = true`).
- **`kill_timeout = "30s"`** on both tasks — a deliberate window over the
  app's `APP_SHUTDOWN_TIMEOUT` (default **10s**): the process gets 10s to
  drain connections gracefully, and Nomad waits up to 30s before SIGKILL.
- **Resources**: api `500 MHz / 384 MiB`, worker `250 MHz / 256 MiB`;
  `restart { attempts = 3, interval = "10m", mode = "delay" }` on both.
- `APP_COOKIE_SECURE=1`, `APP_LOG_FORMAT=json`, `APP_LOG_LEVEL=info` —
  production defaults; no `APP_DEMO_*` anywhere.

## How deploys flow

Both paths build once and deploy the same immutable tag; the job spec's
update block is the safety net either way.

```
push tag v1.x.y
      │
      ├── Woodpecker (primary on your VPS, .woodpecker/build-deploy.yml):
      │     build → Trivy gate → push to 127.0.0.1:5000
      │     → nomad job run -var image_tag=… -var app_domain=…
      │       (BLOCKING: exits non-zero unless the canary passed
      │        /health/ready and was promoted; auto_revert has
      │        already rolled back otherwise)
      │     → smoke check against https://$APP_DOMAIN/health/ready
      │
      └── Ansible (manual or CI-less):
            make vps-app-deploy TAG=v1.x.y
            → deploy-app.yml → nomad job run with the same -var set
```

Full pipeline mechanics and the one-time Woodpecker setup:
[ci-cd.md](ci-cd.md) and [vps/deployment.md](vps/deployment.md) (CI setup
section).

### Manual rollback

```bash
make vps-rollback   # revert the app to the previous successful deployment
```

Because `auto_revert` handles unhealthy deploys automatically,
`vps-rollback` is for the rarer case: a *healthy* deploy you want gone
("the feature works, the product decision was wrong").

## Nomad or Kubernetes?

Short version: Kubernetes + Argo Rollouts is the primary path because its
rollback gate is **metric-driven** — the AnalysisTemplate polls the same
Prometheus SLO queries the alerting layer uses, so a revision serving 200s
at 5× latency fails the p95 gate. Nomad's gate is health-check grade:
`/health/ready` answers "did it start", not "are users having a good time",
and `auto_revert` acts on that. Choose Nomad when the operational budget
cannot carry a control plane, or you already run the HashiStack; accept the
gate or wire metric polling yourself.

The full comparison table and the Swarm verdict:
[orchestrators.md](orchestrators.md).
