# deploy/helm/ — Helm chart (`go-template/`)

Deploys the api and worker Deployments (or Argo Rollouts canary) to Kubernetes.

- `go-template/values.yaml` — knobs of interest: `image`, `config` + `secrets`, `api`/`worker` sizing, `rollout` (canary steps), `ingress`, `hpa`, `podDisruptionBudget`.
- `go-template/templates/` — rollout/deployment, canary + stable services, ingress, HPA, PDB, secret, AnalysisTemplate gating the canary on SLO metrics.

No databases are bundled — provision Postgres/Redis/etc. separately and wire them via `config`.

Docs: [../../docs/deployment/kubernetes.md](../../docs/deployment/kubernetes.md)
