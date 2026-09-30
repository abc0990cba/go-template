# Orchestrators: Kubernetes vs Nomad vs Swarm

The repo ships deployment artifacts for two orchestrators and documents
why not a third. This page is the decision trail — the same "compare,
score, recommend" shape as [the architecture comparison](architecture/17-architecture-options.md).

## What ships

| Path | Artifact | Canary | Auto-rollback gate |
|---|---|---|---|
| Kubernetes | `deploy/helm/go-template` (`rollout.enabled=true`) + `deploy/gitops/` | Argo Rollouts canary 20→50→100% | Prometheus SLO metrics (availability, p95) via AnalysisTemplate |
| Nomad | `deploy/nomad/go-template.nomad.hcl` | `update { canary = 1, auto_promote }` | HTTP health checks (`/health/ready`) + `auto_revert` |
| Docker Swarm | *not shipped* — see below | rolling updates | container health checks |

## Comparison

| Dimension | Kubernetes (+Argo Rollouts) | Nomad | Swarm |
|---|---|---|---|
| Install weight | Heavy: API server, etcd, CNI, then Rollouts controller | One ~100 MB binary; dev agent runs on a laptop | Built into Docker |
| Canary granularity | Real traffic percentages (with ingress/mesh), replica split by default | Replica split (canary allocations) | Replica split |
| **Metric-driven rollback** | **First-class** — analysis queries any Prometheus metric | Not native — gate is health checks; glue via Consul/Nomad tooling | Not available |
| SLO integration | AnalysisTemplate reuses `deploy/prometheus/rules/slo.yml` thresholds | Manual wiring | None |
| Autoscaling | HPA (shipped: CPU 70%, 2–10 replicas) | Nomad Autoscaler (separate) | Swarm has none |
| Ecosystem gravity | The default; every tool targets it first | Healthy niche; HashiStack (Consul/Vault) pairs | Maintenance mode; Docker Desktop deprioritized |
| Local teaching loop | kind (~5 min bootstrap, `make kind-up`) | `nomad agent -dev` (~seconds) | `docker swarm init` (~seconds) |
| When it wins | You want SLO-gated progressive delivery and the broadest ecosystem | You want orchestration without Kubernetes' weight, or already run HashiStack | Tiny fleets already on it; no new commitment advised |

## The verdict this repo encodes

1. **Kubernetes + Argo Rollouts is the primary path** because the decisive
   feature is the *rollback trigger*: canary analysis against the same
   SLO metrics the alerting layer uses (ADR 0024). Health checks — what
   Nomad and Swarm gate on — answer "did the process start", not "are
   users having a good time". A revision that serves 200s at 5× latency
   passes every health check and fails the p95 gate.
2. **Nomad ships as a real alternative**, not a token: the job file is
   complete, validated, and walkable locally in seconds. Choose it when
   the operational budget cannot carry a control plane; accept the
   health-check-grade gate or wire metric polling yourself.
3. **Swarm is documented, not shipped**: it remains useful knowledge
   (and the zero-install demo), but no metric gates and a shrinking
   ecosystem make it a poor recommendation for a template meant to teach
   production practice.

## Rehearse both

```sh
# Kubernetes canary with SLO-gated auto-rollback (kind, ~5 min)
make kind-up
make kind-release   # watch: kubectl -n go-template argo rollouts get rollout go-template-api --watch

# Nomad canary with health-gated auto-revert (single dev agent)
make nomad-up
make nomad-run      # watch: http://localhost:4646
```
