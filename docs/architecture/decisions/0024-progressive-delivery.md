# ADR 0024 — Progressive Delivery: Metric-Gated Canary (Argo Rollouts, Nomad Alternative)

**Status:** Accepted

## Context

The release pipeline ended at the registry: image and chart published,
nothing deployed (a deliberate stop in ADR 0018's scope). The deployment
story was a Helm install with `maxUnavailable: 0` rolling updates — safe
against capacity loss, blind to health: a bad revision rolls out fully as
long as its pods pass readiness. "Faster releases" and "always own the
code" both need the missing half: a deployment that proves itself on
production signals and undoes itself when it doesn't.

## Decision

**Primary path — Kubernetes + Argo Rollouts.** `rollout.enabled=true`
swaps the api Deployment for a `Rollout` (same pod spec, extracted into a
shared Helm helper so the shapes cannot drift). Canary steps
20% → analyze → 50% → analyze → 100%; each analysis is an
`AnalysisTemplate` polling Prometheus for the *same* SLO metrics the
alerting layer uses — availability ≥ 99%, p95 ≤ 500 ms — and the first
breach aborts back to stable (`failureLimit: 0`). No-data (a quiet
canary) counts as success. The HPA target follows the workload kind;
a canary Service twin exists for Rollouts to pin. A GitOps handoff
(`deploy/gitops/application.yaml`) lets Argo CD apply chart releases
without a human `helm upgrade`.

**Alternative — Nomad.** `deploy/nomad/go-template.nomad.hcl` runs the
same two binaries with canary=1, `auto_promote`, and `auto_revert`: one
canary allocation beside stable, `/health/ready` as the gate, automatic
revert on failure. Lighter (a single dev agent runs locally), and its
rollback trigger is health checks rather than business metrics — the
comparison doc (`docs/orchestrators.md`) maps the trade.

**Local rehearsal:** `scripts/kind-demo.sh` (make kind-up / kind-release /
kind-down) boots kind + Prometheus + Argo Rollouts and walks a canary
including the abort path.

## Consequences

**Positive:** shipping is no longer a bet — a revision that degrades the
SLOs is pulled automatically within one analysis window; the gate reuses
the monitoring vocabulary from ADR 0022 (one source of thresholds);
"who deployed what when" becomes `kubectl argo rollouts status` and the
Argo CD UI instead of tribal memory.

**Negative:** Rollouts + Prometheus are new moving parts (the demo
script installs both); analysis windows lengthen releases by minutes —
the explicit price of automatic rollback; Nomad's health-check gate
cannot see SLO regressions that keep pods healthy; trafficRouting (real
percentage splits via ingress-nginx) is wired but off by default, so the
default gate watches replica-split canaries only.

## Alternatives considered

- *Kubernetes Deployments + Flagger* — equivalent canary/analysis
  feature set; Rollouts' CLI and its visibility (`argo rollouts
  get rollout --watch`) made the teaching loop tighter.
- *Docker Swarm rolling updates* — health-check-rollback only, no metric
  gates, effectively maintenance-mode ecosystem; kept as a comparison
  row, not a shipped path.
- *Blue-green* — instant switch but 2× capacity and a same-shaped cliff:
  all users hit the new revision at once. Canary is the safer default
  for a template.
