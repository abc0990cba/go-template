# deploy/ansible

One-VPS production deployment: Ansible provisions, Nomad orchestrates.

- **Start here:** [docs/vps/QUICKSTART.md](../../docs/vps/QUICKSTART.md)
  (`make vps-setup` once, then `make vps-deploy`)
- **Reference:** [docs/vps/deployment.md](../../docs/vps/deployment.md)
- **Operations:** [docs/vps/runbook.md](../../docs/vps/runbook.md)

Cheat sheet:

```bash
make vps-setup         # once per VPS: questions -> secrets -> vault -> inventory
make vps-doctor        # check prereqs / SSH / DNS / VPS size
make vps-deploy        # full converge (idempotent)
make vps-app-deploy TAG=v1.2.3
make vps-rollback
make vps-status | vps-logs S=api | vps-ui | vps-backup | vps-restore FILE=...
```

Never commit: `inventories/*/group_vars/vps/secrets.yml`, `inventories/*/hosts.yml`,
`inventories/*/group_vars/vps/site.yml`, `.vault-pass`, `.vps-state/` (all gitignored).
