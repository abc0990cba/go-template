# deploy/nomad/ — Nomad job specs

- `go-template.nomad.hcl` — local demo job (single dev agent, Docker driver): canary + auto-revert via Nomad's native `update` block.
- `app.nomad.hcl` — **production** job spec and the single source of truth for app deploys; consumed unchanged by both Ansible (`make vps-deploy`) and the Woodpecker pipeline on the VPS.

Docs: [../../docs/deployment/nomad.md](../../docs/deployment/nomad.md)
