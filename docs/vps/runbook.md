# VPS Operations Runbook

Day-2 operations for the single-VPS deployment. Setup is the
[quickstart](QUICKSTART.md); the full map is the
[deployment reference](deployment.md).

Every command below runs from the repo root. Add `VPS_SITE=dev` to operate on
the dev VPS instead of prod.

---

## Deploy

```bash
# Full converge (base system + all jobs + smoke check). Idempotent — safe
# anytime; this is also how config/main.yml changes get applied:
make vps-deploy

# App-only deploy of a specific tag (image must exist in the registry — the
# normal way is to push a v* tag and let CI build it):
make vps-app-deploy TAG=v1.2.3

# Dry run — see everything that would change:
make vps-check

# Re-render + redeploy selected jobs only (after changing main.yml/secrets):
make vps-update JOBS=caddy
make vps-update JOBS=woodpecker     # after rotating CI secrets
make vps-update JOBS=monitoring
```

An api deploy runs a **canary** first; it must pass `/health/ready` before
promotion. If health never turns green, Nomad **auto-reverts** to the
previous version — a failed deploy cannot take the service down.

## Rollback

```bash
make vps-rollback     # instantly reverts the app to the last successful deployment
```

Watch it: `make vps-status` or the Nomad UI (`make vps-ui` →
`http://127.0.0.1:4646`, token: `ssh <vps> sudo cat /etc/nomad.d/tokens/bootstrap.token`).

To roll back further, pick a version from the Deployments table in the UI and
revert to it: `ssh <vps> sudo nomad-admin job revert go-template <version>`.

## Status, logs, admin surfaces

```bash
make vps-status                 # nomad node + job status
make vps-logs S=api             # follow logs: api | worker | postgres | redis | caddy | registry | woodpecker
make vps-ui                     # tunnels: Nomad UI :4646, Grafana :3001
```

Container-level inspection (the Nomad-managed containers are plain docker):

```bash
ssh <vps>
sudo docker ps                  # alloc-<id>-<task> names
sudo nomad-admin job status go-template
sudo nomad-admin alloc status <alloc-id>     # events, restarts, placement
```

Nomad admin CLI shortcut on the VPS: `sudo nomad-admin <subcommand>` — TLS CA
and management token are prewired.

## Backups

```bash
make vps-backup                 # run one now + list recent archives
ssh <vps> ls -lht /opt/go-template/backups
```

Nightly at `backup_at` (default 03:15) via systemd
(`go-template-backup.timer`), keeping 7 days (configurable). Archives:
`go_template-*.dump`, `woodpecker-*.dump`, `secrets-and-tls-*.tgz`.

Off-site (recommended): set `backup_rclone_remote` + `backup_rclone_config`
in `secrets.yml`, then `make vps-deploy` (re-renders secrets and installs the
rclone config) — backups start pushing automatically.

### Restore a backup

**Drill this once before you need it.** Restores are destructive for the
target database by design.

```bash
# from your workstation, using any dump (local copy or scp'd from the VPS):
make vps-restore FILE=go_template-YYYYMMDD-HHMMSS.dump
# or a specific database: make vps-restore FILE=woodpecker-....dump woodpecker
```

The script stops the app job, recreates the database, `pg_restore`s, restarts
the app, and waits for `/health/ready`.

Full node loss: provision a fresh VPS → `make vps-setup` (same answers; keep
the old `secrets.yml` + `.vault-pass` to preserve identities) → `make
vps-deploy` with CI disabled → `make vps-restore FILE=<latest dump>` →
re-enable CI by restoring the OAuth client + Woodpecker secrets.

## Rotate a secret

1. Edit: `ansible-vault edit deploy/ansible/inventories/prod/group_vars/vps/secrets.yml`
2. Re-render + restart what consumes it:
   - app secrets (jwt, db, smtp): `make vps-deploy` (re-renders `/secrets` and
     rolls the app with canary)
   - CI secrets: `make vps-update JOBS=woodpecker` (then update the matching
     Woodpecker repo secrets in the UI)
   - Grafana admin: `make vps-update JOBS=monitoring`
3. Verify, then commit nothing (secrets are gitignored by design).

Rotate the Nomad management token: create a new one
(`ssh <vps> sudo nomad-admin acl token create -policy=... -name=rotated`),
switch consumers, then delete the old. The `deploy` (CI) token rotates the
same way — update the Woodpecker secret afterwards.

## Switch Let's Encrypt staging → production

```bash
make vps-acme-production
```

Do this once DNS + firewall are proven (doctor passes, app reachable with the
staging cert warning). Caddy re-issues against the production CA
automatically.

## Monitoring & alerting

- **Dashboards**: `make vps-ui` → `http://127.0.0.1:3001` (login `admin`,
  password in `secrets.yml`) — the repo's API/worker/resilience dashboards
  are pre-provisioned.
- **Alerts**: firing alerts are always visible in the Alertmanager UI
  (tunnel: `ssh -L 9093:127.0.0.1:9093 <vps>` → `http://127.0.0.1:9093`).
  Wire a receiver: set `alert_receiver: telegram|slack|email` in `main.yml` +
  credentials in `secrets.yml`, then `make vps-update JOBS=monitoring`.
- Rules and dashboards come from the repo (`deploy/prometheus/rules`,
  `deploy/grafana`) — change them in a PR like any other code; `make
  vps-deploy` ships them.

## Node maintenance (reboots, resizing, migration)

```bash
# planned reboot: Nomad + docker come back on boot; allocations restore.
ssh <vps> sudo reboot
```

Migrate to a bigger VPS: provision it → `vps-setup` (reuse old
`secrets.yml`/`.vault-pass` for identical secrets) → `vps-deploy` →
`vps-restore` with the latest dump. Data lives in `/opt/go-template/data`
(Postgres is the only stateful component that matters; rsync it for a
zero-dump migration while the old node is stopped).

## Troubleshooting

| Symptom | Diagnosis |
|---|---|
| `vps-deploy` fails at "Wait for https://.../health/ready" | `make vps-logs S=api`; check `nomad-admin job status go-template`; 90% of the time it is DNS not pointing at the VPS yet (cert issuance pending) |
| Certificate errors in the browser | still on the LE staging CA (expected pre-`vps-acme-production`), or DNS changed recently — check `make vps-doctor` |
| CI pipeline fails at the deploy step with TLS/x509 error | `nomad_ca_cert` Woodpecker secret stale — re-set it from `/etc/nomad.d/tls/ca.pem` after a cert re-sign |
| CI pipeline fails at deploy with `Permission denied` | `nomad_token` secret missing/revoked — re-set from `/etc/nomad.d/tokens/deploy.token` |
| Postgres won't start after a crash | `ssh <vps> sudo nomad-admin alloc status -job postgres`; logs via `make vps-logs S=postgres`; data is in `/opt/go-template/data/postgres` |
| Disk filling up | old images: `ssh <vps> sudo docker image prune -af`; registry: wait for Sunday GC or run `/usr/local/sbin/go-template-registry-gc`; Prometheus retention: lower `prometheus_retention` + `vps-update JOBS=monitoring` |
| Bootstrap build job failed | `ssh <vps> sudo nomad-admin job status build-bootstrap` — usually a private `bootstrap_repo_url` or a transient registry/network hiccup; fix and re-run `make vps-deploy` |
| Locked out over SSH? | the `deploy` user + your key were provisioned in the same run that disabled root; use your provider's recovery console if both fail |

### Recover a lost Nomad management token

If `/etc/nomad.d/tokens/bootstrap.token` is gone but ACLs were bootstrapped,
generate a new management token via `nomad acl token update` using any
surviving token — or reset the bootstrap index (`nomad-admin acl bootstrap`
will then fail with "already done" pointing you to `acl reset`) using the
ResetIdx from `nomad-admin acl bootstrap` output. Worst case, stop Nomad,
temporarily set `acl { enabled = false }` in `/etc/nomad.d/nomad.hcl`,
restart, recreate tokens, restore the config — single-node blast radius is
contained by the loopback + UFW posture.

## Upgrade / bump versions

All versions pin in `deploy/ansible/inventories/prod/group_vars/vps/main.yml`
(`nomad_version`, `docker_apt_version`, every container image). Bump the var,
run `make vps-deploy`. Watch the job's deployment in the Nomad UI; Nomad
auto-reverts an unhealthy roll. Read upstream release notes before jumping a
major (Postgres majors need a dump/restore — use `vps-restore`).

## Teardown

Everything lives under `/opt/go-template` (+ `/opt/nomad`, `/etc/nomad.d`,
two systemd timers, UFW rules, two apt repos). To decommission the VPS:
destroy it at the provider and remove the local site state:

```bash
rm -rf deploy/ansible/.vps-state deploy/ansible/inventories/prod/hosts.yml \
       deploy/ansible/inventories/prod/group_vars/vps/{site.yml,secrets.yml}
```
