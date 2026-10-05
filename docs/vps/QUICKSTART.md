# VPS Quickstart: zero → HTTPS in ~20 minutes

One VPS, one command. This guide takes a **fresh Ubuntu 24.04 VPS** and your
**domain name** to a fully deployed, TLS-secured stack — app + Postgres + Redis
+ CI + monitoring + nightly backups — provisioned by Ansible, orchestrated by
Nomad.

The full explanation of every moving part lives in
[deployment.md](deployment.md); day-2 operations in the
[runbook](runbook.md).

---

## 0. Prerequisites (5 minutes, one time)

> **Want to see the whole flow first, without a VPS?** `make vps-local`
> rehearses this entire deployment on an OrbStack Ubuntu machine on your Mac —
> same playbooks, ~95% fidelity, deletable in one command.
> See [LOCAL-REHEARSAL.md](LOCAL-REHEARSAL.md).

1. **A VPS** — Ubuntu **24.04 LTS** (22.04 works), 2+ vCPU, **8 GB RAM
   recommended** (4 GB minimum: see [sizing](deployment.md#sizing)), from any
   provider. Create it with your SSH key.
2. **DNS A records** pointing at the VPS IP (or one wildcard `*.example.com`):

   | Record | Example          | Purpose              |
   |--------|------------------|----------------------|
   | A      | `app.example.com`  | the application      |
   | A      | `ci.example.com`   | Woodpecker CI (self-hosted) |
   | A      | `grafana.example.com` | monitoring dashboards |

3. **Workstation tooling** — `git` and an SSH key you already have. Ansible is
   the only extra dependency; `make vps-setup` tells you the exact install
   command for your OS if it's missing (macOS: `brew install ansible`,
   Debian/Ubuntu: `sudo apt install ansible-core`).

## 1. Setup (once per VPS)

```bash
git clone <your-fork> go-template && cd go-template
make vps-setup
```

You will answer **six questions** (VPS IP, SSH user, base domain, Let's
Encrypt email, repo URL, what to enable). Everything else is generated for
you: all secrets, an encrypted vault, and the inventory. Secrets never leave
your machine unencrypted (`deploy/ansible/inventories/*/group_vars/vps/secrets.yml`
is ansible-vault encrypted; the vault password sits in the gitignored
`deploy/ansible/.vault-pass`).

## 2. Verify (optional but recommended)

```bash
make vps-doctor
```

Checks ansible, SSH reachability, DNS records, and VPS size — prints exact
fix instructions for anything red.

## 3. Deploy (the one command)

```bash
make vps-deploy
```

~10–20 minutes on the first run. It hardens the VPS (firewall, fail2ban,
key-only SSH, auto security updates), installs pinned Docker + Nomad (mTLS +
ACLs), brings up Postgres → Redis → registry → builds the first image on the
VPS → deploys the app behind **Caddy with automatic Let's Encrypt
certificates** → Woodpecker CI → Prometheus/Grafana → nightly backups, and
ends with a smoke check against `https://app.example.com/health/ready`.

> First run issues **Let's Encrypt staging** certificates (no rate limits).
> Once everything is green, switch to production certs:
> `make vps-acme-production`

Re-running `make vps-deploy` is always safe — it converges and deploys
updates (api deploys are canary + auto-revert, gated on `/health/ready`).

## 4. Verify the deployment

| Check | Command |
|---|---|
| App is healthy | `curl -fsS https://app.example.com/health/ready` |
| Nomad jobs all running | `make vps-status` |
| Dashboards | `make vps-ui` → open `http://127.0.0.1:3001` (login `admin`, password in `secrets.yml`) |
| First backup | `make vps-backup` |

## 5. Ship your first release through your own CI

One-time (10 minutes, [detailed guide](deployment.md#ci-setup)):

1. Create a **GitHub OAuth app** → callback `https://ci.example.com/oauth2/response`;
   put the client id/secret into `secrets.yml`
   (`ansible-vault edit deploy/ansible/inventories/prod/group_vars/vps/secrets.yml`)
   then `make vps-update JOBS=woodpecker`.
2. Log into `https://ci.example.com`, **activate the repository**.
3. Add three **Woodpecker secrets** (enabled for the `tag` event):
   - `nomad_token` — value of `/etc/nomad.d/tokens/deploy.token` on the VPS
   - `nomad_ca_cert` — value of `/etc/nomad.d/tls/ca.pem`
   - `app_domain` — `app.example.com`

From now on every pushed tag ships itself:

```bash
git tag v1.0.0 && git push origin v1.0.0   # build → security scan → deploy → smoke
git push origin :refs/tags/v1.0.0 && git tag -d v1.0.0   # rollback path if ever needed:
make vps-rollback                          # instant revert to the previous deployment
```

---

## What you have now

- **App** at `https://app.example.com` — one Go binary serving the API + the
  embedded React SPA, canary deploys with auto-rollback.
- **Your own CI/CD** at `https://ci.example.com` — builds on your VPS, pushes
  to your own registry, deploys via Nomad. GitHub Actions/GHCR remain as the
  documented fallback path.
- **Monitoring** — Prometheus + Alertmanager + Grafana with the repo's
  dashboards and SLO burn-rate alerts; wire a Telegram/Slack/email receiver
  with one variable ([how](deployment.md#alert-delivery)).
- **Backups** — nightly `pg_dump` + secrets archive, 7-day retention,
  off-site push optional; [restore drill](runbook.md#restore-a-backup).
- **Hardening** — key-only SSH (no root), fail2ban, UFW (22/80/443 only),
  unattended security upgrades, loopback-only data stores, Nomad mTLS + ACLs,
  secrets as root-only files consumed via `APP_*_FILE`.

## Common first-run hiccups

| Symptom | Fix |
|---|---|
| `make vps-setup` says ansible missing | install per the printed command, re-run |
| Deploy fails at certificate issuance | DNS not propagated — `make vps-doctor`, wait, re-run `make vps-deploy` |
| 525/timeout from `https://app...` | still on staging certs (expected) — run `make vps-acme-production` |
| CI shows no repos | OAuth app not configured yet — step 5.1 above |
| Anything else | [runbook troubleshooting](runbook.md#troubleshooting) |
