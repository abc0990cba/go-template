# deploy/ — deployment assets hub

Everything needed to ship the api + worker pair, from a single VPS to GitOps-managed Kubernetes.

| Path | What it is |
| --- | --- |
| `ansible/` | VPS provisioning and deploys (roles for Docker, Nomad, backups; dev/local/prod inventories) |
| `helm/` | Helm chart for Kubernetes with Argo Rollouts canary + SLO-gated analysis |
| `nomad/` | Nomad job specs: local demo walkthrough and the production job |
| `gitops/` | Argo CD app-of-apps manifest |
| `prometheus/` | Alert and SLO recording/burn-rate rules |
| `prometheus.yml` | Root Prometheus scrape config (compose stack) |
| `grafana/` | Provisioned datasources and dashboards |
| `alertmanager/` | Alert routing tree |

## Which path do I pick?

Start with **[../docs/deployment/README.md](../docs/deployment/README.md)** — it compares the options and routes you to the right guide. For the fastest path to a running VPS, follow the quickstart: [../docs/deployment/vps/QUICKSTART.md](../docs/deployment/vps/QUICKSTART.md).

## Single source of truth

`deploy/nomad/app.nomad.hcl` is the production app spec consumed **unchanged** by both deploy paths: Ansible (`make vps-deploy`) and the Woodpecker CI pipeline on the VPS.
