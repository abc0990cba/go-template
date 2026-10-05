# VPS Deployment Reference

The complete map of the single-VPS production deployment: what runs where,
why each decision was made, every knob, and how CI/CD, monitoring and backups
fit together. Day-2 operations live in the [runbook](runbook.md); the short
path from zero is the [quickstart](QUICKSTART.md).

---

## Architecture

```
                         Internet
                            │
                 ┌──────────▼───────────┐        VPS (Ubuntu 24.04, one node)
                 │  UFW: 22, 80, 443    │
                 └──────────┬───────────┘
                            │
              ┌─────────────▼──────────────┐
              │ Caddy (Nomad job)          │  automatic Let's Encrypt,
              │ :80/:443, TLS termination  │  auto-renew, HTTP→HTTPS
              └──┬─────────┬─────────┬─────┘
   app.example.com│  ci.example.com  grafana.example.com
                 ▼                ▼         ▼
        ┌─────────────┐   ┌───────────┐  ┌─────────┐
        │ api :8080   │   │Woodpecker │  │ Grafana │   everything below
        │ (api+SPA)   │   │server:8000│  │  :3000  │   binds 127.0.0.1
        └──┬────┬─────┘   └─────┬─────┘  └────┬────┘   and is firewalled
           │    │               │             │
           │ ┌──▼─────────┐ ┌───▼──────────┐ ┌▼─────────────────────┐
           │ │Postgres    │ │registry :5000│ │Prometheus :9091      │
           └►│ :5432      │ └───┬──────────┘ │Alertmanager :9093    │
             └──┬─────────┘     │ images     │node/blackbox exporters│
           ┌──▼─────────┐      ▼            └──────────┬───────────┘
           │Redis :6379 │ ┌──────────────┐             │ scrapes
           └────────────┘ │Woodpecker    │◄────────────┘ (blackbox probes
             ▲            │agent (docker)│              the public URL)
     ┌──────┴───────┐     └──────┬───────┘
     │ worker :9090 │◄───────────┘ deploys via Nomad API (mTLS + ACL token)
     └──────────────┘
                            ┌──────────────────────────────┐
                            │ systemd: nightly pg_dump,    │
                            │ weekly registry GC           │
                            └──────────────────────────────┘
```

Everything is a **Nomad job** (one Nomad agent = server + client on the same
node, [mTLS + ACLs](#security-model)). Provisioning is **Ansible**
(`deploy/ansible/`) driven by one command; there is no k8s, no Consul, no
Vault — deliberately (rationale: [docs/orchestrators.md](../orchestrators.md)
already positions Nomad as the low-operations-budget path; the same repo also
ships the Helm/Argo path for when you outgrow one node).

> **Rehearse before you fly:** the identical flow runs locally on an OrbStack
> Ubuntu machine — `make vps-local`. See
> [LOCAL-REHEARSAL.md](LOCAL-REHEARSAL.md) for what differs (internal TLS,
> `hardening_level: light`, working-tree image builds).

### Component inventory

| Job / unit | Image (pinned in `group_vars/vps/main.yml`) | Binds | Purpose |
|---|---|---|---|
| `caddy` | `caddy:2.10.2` | :80/:443 public | TLS termination, routing, security headers, blocks `/metrics` + `/debug` at the edge |
| `go-template` (api) | `127.0.0.1:5000/go-template:<tag>` | :8080 | API + embedded SPA + `/metrics`; canary + auto-revert deploys |
| `go-template` (worker) | same image, `/app/worker` | 127.0.0.1:9090 | asynq jobs, cron, outbox relay, stream consumers |
| `postgres` | `postgres:16.9` | 127.0.0.1:5432 | the only stateful source of truth; migrations run at api startup |
| `redis` | `redis:7.4.5-alpine` | 127.0.0.1:6379 | queue/cache/rate-limits; loopback-only because the app's Redis client has no auth support |
| `registry` | `registry:2.8.3` | 127.0.0.1:5000 | your own OCI registry (primary image store) |
| `woodpecker` (server+agent) | `woodpeckerci/woodpecker-{server,agent}:v3` | 127.0.0.1:8000/9000 | self-hosted CI; builds run in docker on the VPS |
| `monitoring` (5 groups) | prometheus v3.4.1, alertmanager v0.28.1, grafana 11.6.0, node-exporter v1.9.1, blackbox v0.26.0 | 127.0.0.1:* | metrics, alert routing, dashboards, host + synthetic probes |
| `build-bootstrap` (one-shot) | `docker:28` | — | first-deploy-only image build before CI exists |
| systemd `go-template-backup.timer` | — | — | nightly pg_dump + secrets/TLS archive, retention, optional rclone |
| systemd `go-template-registry-gc.timer` | — | — | weekly registry garbage-collect |

## Repository layout

```
deploy/
  ansible/                  ← everything Ansible (run via make vps-*)
    ansible.cfg             vault-password wiring, ssh pipelining
    site.yml                full converge (make vps-deploy)
    deploy-app.yml          subset deploy (make vps-app-deploy / vps-update)
    inventories/prod/       hosts.yml + group_vars/vps/{main,site,secrets}.yml
    inventories/dev/        Phase-2 scaffold for a second (dev) VPS
    roles/common/           ssh hardening, ufw, fail2ban, unattended-upgrades, dirs
    roles/docker/           pinned docker-ce, log rotation, wpnet
    roles/nomad/            pinned nomad, internal CA + mTLS, ACL bootstrap + policies
    roles/nomad_jobs/       renders configs+secrets, validate→run→health-gate per job
    roles/backup/           systemd timers (nightly dump, weekly GC)
    scripts/                vps-setup, vps-doctor, run-playbook, rollback, ui, restore
  nomad/app.nomad.hcl       app job spec — SHARED by Ansible and the CI pipeline
.woodpecker/build-deploy.yml   primary CI/CD pipeline (runs on your VPS)
```

## Security model

Layered, "strict from day 1":

- **SSH**: key-only, root login disabled, `AllowUsers` allowlist, fail2ban
  (1h ban after 5 failures). The playbook creates a `deploy` sudo user and
  automatically switches the inventory to it after the first run.
- **Firewall (UFW)**: default-deny incoming; only 22, 80, 443 public. Nomad's
  4646 additionally allows docker bridge ranges only (for CI deploys).
- **Network**: every service except Caddy binds `127.0.0.1` (host-network
  containers). No docker published ports → no iptables/UFW bypass class of
  bugs. Postgres/Redis are unreachable from outside the host by construction.
- **Nomad**: mTLS with an Ansible-managed internal CA (10y CA, 825d node cert
  with auto re-sign 90 days before expiry); **ACLs enabled** — a management
  token lives root-only on the VPS (`/etc/nomad.d/tokens/bootstrap.token`),
  CI uses a least-privilege `deploy` policy token. `sudo nomad-admin …` is
  the prewired CLI.
- **Secrets**: generated by `vps-setup`, stored **ansible-vault encrypted**
  in the repo tree (gitignored), rendered on the VPS as root/uid-owned `0400`
  files under `/opt/go-template/secrets/` and consumed via the app's native
  `APP_*_FILE` pattern. Secrets never appear in images, git, or job env for
  the app; the Woodpecker job file (which carries CI secrets) is `0600`.
- **Supply chain**: images pinned by tag; CI pipeline blocks CRITICAL/HIGH
  (fixable) CVEs via Trivy before push; the base app image is distroless.
- **OS**: unattended-upgrades applies security updates daily; automatic
  reboots stay off (a reboot is a human decision on a single node).

### Optional hardening (deliberately deferred)

Off by default, all documented as follow-ups: Nomad node cert renewal
alerting, `verify_server_hostname` pinning, Vault server instead of file
secrets, WAF/rate-limit at the edge (the app already rate-limits), and
provider-level cloud firewall in addition to UFW.

## Configuration knobs

All in `deploy/ansible/inventories/prod/group_vars/vps/`:

- **`main.yml`** (committed defaults): domains, ACME email + staging flag,
  `ci_enabled`, `monitoring_enabled`, `alert_receiver`, image pins, replica
  count, backup schedule, optional SMTP.
- **`site.yml`** (gitignored, written by `vps-setup`): machine-specific
  values — your domains, email, repo URL.
- **`secrets.yml`** (ansible-vault encrypted, gitignored): every secret —
  see `secrets.yml.example` for the annotated catalog.

Environment separation: point any `vps-*` target at a second inventory with
`VPS_SITE=dev` (scaffold + recommended profile in
`deploy/ansible/inventories/dev/`).

### Sizing

| Profile | VPS | Settings |
|---|---|---|
| Full (recommended) | 2 vCPU / 8 GB | defaults (CI + monitoring on) |
| Lean | 2 vCPU / 4 GB | `ci_enabled: false`, keep monitoring; build images via GHCR fallback or a second machine |
| App-only dev box | 1 vCPU / 2 GB | `ci_enabled: false`, `monitoring_enabled: false` |

Builds are the RAM-heavy part (`WOODPECKER_MAX_WORKFLOWS=1` caps concurrency).

## CI/CD

Two paths, one artifact flow — **build once, deploy the same immutable tag**:

1. **Primary — your own infra** (`.woodpecker/build-deploy.yml`, runs on the
   VPS): on a `v*` tag → `docker build` (VERSION/COMMIT/DATE stamped) →
   **Trivy gate** (fails on CRITICAL/HIGH with a fix available) → push to
   your local registry (`127.0.0.1:5000`, semver + sha tags) → `nomad job
   run -var image_tag=…` over the Nomad API with the **least-privilege deploy
   token + mTLS CA** (reachable from the pipeline's docker network, UFW
   scoped) → **post-deploy smoke** against the public `/health/ready`.
2. **Fallback — GitHub** (`.github/workflows/release.yml`, untouched): tag →
   multi-arch image to GHCR + cosign/SBOM/SLSA. To consume it, set
   `app_image: ghcr.io/<org>/go-template` in `main.yml` and add a
   `docker.config.auth` block / registry mirror for ghcr (runbook) — or just
   `docker pull` + `docker tag` into the local registry.

Deployment safety on both paths: the app job spec
([deploy/nomad/app.nomad.hcl](../nomad/app.nomad.hcl)) declares
`update { canary = 1, auto_promote, auto_revert }` gated on `/health/ready`
— an unhealthy new version never serves traffic and the deployment reverts
automatically. `make vps-rollback` reverts instantly to the previous
successful version.

### CI setup (one-time)

1. **GitHub OAuth app**: `https://github.com/settings/developers` →
   Homepage `https://ci.example.com`, callback
   `https://ci.example.com/oauth2/response`. Put the client id/secret into
   `secrets.yml` (`woodpecker_github_client_id/secret`), then
   `make vps-update JOBS=woodpecker`.
2. Open `https://ci.example.com`, sign in with GitHub (first user becomes
   admin when `woodpecker_admin_user` is empty), **activate the repository**.
3. **Woodpecker repo secrets** (Repository → Secrets, enable *tag* event):
   - `nomad_token` ← `ssh <vps> sudo cat /etc/nomad.d/tokens/deploy.token`
   - `nomad_ca_cert` ← `ssh <vps> sudo cat /etc/nomad.d/tls/ca.pem`
   - `app_domain` ← your `app.example.com`

### Private repos

The one-shot bootstrap build clones `bootstrap_repo_url` without credentials.
For a private repo, either push a first image from your machine
(`docker build && docker push` after an SSH tunnel to the registry) or add a
deploy token to the clone step; the CI path authenticates to GitHub with the
OAuth app, not with repo credentials.

## Observability

Whitebox + blackbox + host, reusing the repo's existing configs so
**dashboards and alerts are versioned with the app**:

- Scrape targets (`configs/prometheus/prometheus.yml`, rendered): api
  `/metrics` (:8080), worker `/metrics` (:9090), node-exporter (:9100), and
  a **synthetic outside-in probe** of `https://app.example.com/health/live`
  through Caddy via blackbox-exporter — the check that catches a broken
  front door while in-process metrics look green.
- Rules: the repo's `deploy/prometheus/rules/{alerts,slo}.yml` (process
  availability, RED, jobs/outbox/dead-letters, resilience, SLO burn-rate).

### Alert delivery

`alert_receiver` in `main.yml` = `none | telegram | slack | email` (+ credentials in
`secrets.yml`). `none` keeps alerts visible in the Alertmanager UI only
(tunnel with `make vps-ui`).
- Grafana: the repo's three dashboards provision automatically; datasource
  uid `prometheus` preserved. Public via `https://grafana.example.com`
  (admin password from `secrets.yml`) or loopback-only via the tunnel.
- Nomad's own metrics are exposed on the API with `prometheus_metrics = true`
  for ad-hoc scraping.

### Log aggregation (Loki + promtail, `monitoring_logs_enabled`, default on)

promtail discovers containers over the Docker socket and ships their logs
to a single-binary Loki (filesystem storage, 7-day retention, loopback
:3100). Grafana gets a `loki` datasource automatically — Explore is the
query UI (`{container="api"} | json`). Prometheus scrapes both pipelines'
own metrics, so a broken log pipeline alerts like any other target. Data
lives in `/opt/go-template/data/loki`; size it for the retention you keep
(7 days of this stack is well under 1 GB).

### Uptime Kuma (`monitoring_uptime_enabled`, default on)

Browser-configured synthetic checks behind `https://uptime.example.com`
(Caddy route, its own account on first visit — set the admin password
immediately). It probes from this host, so treat it as a convenience view:
keep one external probe (Better Stack, Healthchecks.io, anything off-node)
for true outside-in monitoring.

## Backups & disaster recovery

- **What**: nightly `pg_dump -Fc` of `go_template` (+ `woodpecker` when CI is
  on) + a tar of `/opt/go-template/secrets` and Nomad TLS/tokens. Postgres is
  the only stateful source of truth — the app, registry, and TSDB rebuild
  from images/config ([docs/architecture/13](../architecture/13-deployment-and-operations.md)).
- **Where**: `/opt/go-template/backups`, kept `backup_local_retention_days`
  (default 7). Set `backup_rclone_remote` + `backup_rclone_config` in
  `secrets.yml` for off-site (S3/B2/anything rclone speaks) — **recommended**,
  the VPS is a single point of failure.
- **Restore**: `make vps-restore FILE=go_template-YYYYMMDD-HHMMSS.dump`
  (drill documented in the [runbook](runbook.md#restore-a-backup)).
- Backup runs at `backup_at` (default 03:15 — deliberately after the app's
  03:00 report cron).

## Phase 2: dev environment and multi-server reliability

Designed-in seams, implemented when you need them:

- **Dev environment**: `deploy/ansible/inventories/dev/` — same playbooks,
  `VPS_SITE=dev`, recommended profile disables CI (reuse prod-built tags).
- **Multiple replicas on one node**: switch the app job from host networking
  to Nomad bridge networking (CNI plugins + dynamic ports); Caddy's static
  upstreams become Traefik with the Nomad provider (one job-file swap —
  exactly why the proxy is isolated in `caddy.nomad.hcl` + one Caddyfile).
- **Multiple servers**: add Nomad clients (same `nomad` role with
  `bootstrap_expect = 3` servers), introduce Consul for cross-node service
  discovery, keep Postgres on its own node (or managed), point Caddy at the
  api service per node or add a load-balancer node. The inventory model
  already supports multiple hosts per site.
- **Stateful growth**: move Postgres off the app node first; Redis last
  (it is rebuildable cache/queue state).

## Design decisions & trade-offs (FAQ)

- **Why Nomad over k8s here?** One VPS can't carry a control plane; Nomad
  gives canary/auto-revert/health-gates natively. The repo's own ADRs
  ([0024](../architecture/decisions/0024-progressive-delivery.md),
  [docs/orchestrators.md](../orchestrators.md)) document both paths.
- **Why Caddy?** Certificates fully out of the box (obtain/renew/hot-reload),
  3-line config, single Go binary. Traefik's Nomad provider becomes relevant
  only in the multi-node Phase 2 — swap is one job file.
- **Why host networking?** On a single node it removes the docker-vs-UFW
  iptables bypass class entirely, keeps every data store loopback-only, and
  needs no CNI. Cost: replicas-per-node must move to bridge networking in
  Phase 2 (see above).
- **Why a local registry?** Your images live on your infra (primary), GHCR
  remains the vendor fallback; pushes/pulls never leave the VPS.
- **Why not Vault?** The app's `APP_*_FILE` pattern + vault-encrypted
  Ansible secrets covers single-node needs without another service to
  operate; the seam is documented for later.
