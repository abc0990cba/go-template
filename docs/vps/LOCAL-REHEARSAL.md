# Local Rehearsal: the whole deploy on your Mac (OrbStack)

Rehearse the **entire** production deployment flow — Ansible → Nomad → Caddy →
Postgres/Redis/registry → app → Woodpecker CI → monitoring → backups — on an
OrbStack Ubuntu *machine* before touching a real VPS. Same playbooks, same
job specs, same versions; only OS-level hardening and public TLS are adapted
for a laptop.

```bash
make vps-local        # ONE command — first run ≈ 10–20 min, re-runs ≈ 1–2 min
```

---

## Why an OrbStack machine (and not a container)

Your playbooks need four things from a target: SSH, systemd, real apt, and
the ability to run dockerd + Nomad. An OrbStack **machine** is a full Ubuntu
VM — it has all four, with a shared kernel so it stays light on the battery.
Compared to the alternatives:

| Option | Full-flow fidelity | Notes |
|---|---|---|
| **OrbStack machine** (this page) | ~95% | fastest loop on a Mac; one command to create, one to delete |
| Lima / Colima VM | ~95% | free OSS fallback if you drop OrbStack — same design, `limactl` instead of `orb` |
| Multipass VM | ~95% | fine, heavier images |
| UTM/QEMU VM | ~98% | closest to a provider (real cloud-init), slowest loop |
| systemd-in-Docker container | low | no UFW, dockerd-in-docker + Nomad in a container is brittle — role tests only |
| `make vps-check` (dry run) | logic only | complements, never replaces, a rehearsal |

## Prerequisites

- **OrbStack** running: `brew install orbstack` (then open it once). Check: `orb version`.
- `rsync` (macOS ships it) and `ansible-core` — `make vps-doctor` tells you
  exactly what's missing.
- Nothing else: no domain, no DNS, no VPS account.

## The one command

```bash
make vps-local
```

It is idempotent and non-interactive. What happens, in order:

| Phase | What runs | Time |
|---|---|---|
| 0. Machine | `orb create ubuntu:24.04 go-template-vps` (skipped when it exists) | ~1 min, once |
| 1. SSH | generates `.vps-state/local_key`, installs `openssh-server` + your key in the machine | seconds, once |
| 2. Base | `site.yml --skip-tags stack,backup,smoke` — the production `common`/`docker`/`nomad` roles: pinned Docker CE + Nomad with mTLS and ACLs (`hardening_level: light` skips apt-upgrade/fail2ban/unattended-upgrades/UFW) | 3–6 min |
| 3. Image | rsyncs your **working tree** into the machine and `docker build`s `127.0.0.1:5000/go-template:<tag>` there | 5–12 min first, cached afterwards |
| 4. Stack | full `site.yml` — Postgres, Redis, registry, app (canary + auto-revert), Woodpecker, Caddy (`tls internal`), monitoring, backup timers; the image from phase 3 skips the GitHub-clone bootstrap build | 3–6 min |
| 5. Verify | `make vps-verify` — the PASS/FAIL table below | ~10 s |

The run ends with the verification table, the `/etc/hosts` lines, the
optional trust-the-CA command, and the teardown hint.

## Verifying (easy to check, by design)

```bash
VPS_SITE=local make vps-verify
```

Prints a PASS/FAIL table — Nomad job statuses over SSH, plus
`https://app.go-template.test/health/ready`, `/version`, an edge-403 check on
`/metrics`, CI `/healthz`, Grafana `/api/health`. It uses `curl --resolve`
against the machine directly, so it works **even without any `/etc/hosts`
entries**. For a human in a browser, add the names once:

```bash
sudo tee -a /etc/hosts >/dev/null <<'EOF'
127.0.0.1 app.go-template.test ci.go-template.test grafana.go-template.test
EOF
```

(OrbStack forwards machine ports to macOS localhost automatically, so the
names resolve to `127.0.0.1`.) Certificates are signed by Caddy's local CA —
accept the browser warning, or trust the root once (command is printed at the
end of `vps-local`):

```bash
orb -m go-template-vps sudo cat /opt/go-template/data/caddy/pki/authorities/local/root.crt \
  > /tmp/go-template-local-root.crt
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /tmp/go-template-local-root.crt
```

## What differs from production (deliberately)

| Aspect | Production | Local rehearsal |
|---|---|---|
| TLS | Let's Encrypt via Caddy (`tls_mode: acme`) | Caddy internal CA (`tls_mode: internal`) |
| Domains | your real subdomains + DNS A records | `*.go-template.test` + `/etc/hosts` |
| OS hardening | apt-upgrade, fail2ban, unattended-upgrades, UFW, hardened sshd (`hardening_level: full`) | those four skipped (`light`); everything else identical |
| First image | built on the VPS from `bootstrap_repo_url`, then by CI from tags | built in the machine from your **rsynced working tree** — rehearses uncommitted changes |
| Secrets | your prod secrets (vault-encrypted in `inventories/prod/...`) | separate auto-generated vault in `inventories/local/...` |
| CI login | GitHub OAuth app | OAuth callback needs a real public URL, so the login step is skipped — the CI *deploy* path is identical and exercised via `VPS_SITE=local make vps-app-deploy TAG=...` |
| Inventory | `inventories/prod/` (gitignored, written by `vps-setup`) | `inventories/local/` (hosts.yml gitignored, written by `vps-local`) |

Everything between "Ansible playbook" and "HTTP response" — pinned package
versions, Nomad mTLS + ACL tokens, host volumes, job specs, health-gated
canary deploys, Caddyfile, monitoring wiring, backup timers — is the **same
code path** as production.

## Daily rehearsal loop

```bash
# change code, then rehearse the full flow:
make vps-local                                  # or, after the first run:
VPS_SITE=local make vps-deploy                  # full converge, ~1–2 min

# rehearse just an app release (what CI's deploy step does):
VPS_SITE=local make vps-app-deploy TAG=v0.1.0

# operate it exactly like prod:
VPS_SITE=local make vps-status
VPS_SITE=local make vps-logs S=api
VPS_SITE=local make vps-ui                      # tunnels: Nomad UI :4646, Grafana :3001
VPS_SITE=local make vps-rollback
VPS_SITE=local make vps-check                   # dry run
```

Because the working tree is rsynced on every `make vps-local`, whatever you
have uncommitted is exactly what gets built and deployed.

## Teardown / reset

```bash
orb delete go-template-vps                      # removes the "VPS" entirely
rm -rf deploy/ansible/.vps-state                # local key + state
rm -f deploy/ansible/inventories/local/group_vars/vps/secrets.yml
rm -f deploy/ansible/inventories/local/hosts.yml
```

Nothing on your Mac outside `.orb/` and the repo's gitignored files is
touched.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `orb: command not found` | `brew install orbstack`, open OrbStack once, re-run |
| SSH into machine times out | `orb status go-template-vps`; `orb start go-template-vps`; delete + re-run if persistent |
| Phase 3 docker build fails with `docker: command not found` | phase 2 didn't complete — check its ansible output; re-run `make vps-local` (idempotent) |
| Browser can't reach `https://app.go-template.test` | missing `/etc/hosts` entry (see above) or OrbStack not running; `VPS_SITE=local make vps-verify` always works without hosts entries |
| `x509` warnings | expected with the internal CA — accept, or trust the root cert (command above) |
| Woodpecker shows no repos | expected locally (no GitHub OAuth callback); use `vps-app-deploy` to rehearse deploys |
| Machine eats disk | `orb delete go-template-vps` and re-create; docker cache lives in the machine |
| Port 443 already used on the Mac | stop the other listener; OrbStack forwards machine ports to localhost |
