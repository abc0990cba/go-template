## Why

The release pipeline stopped at the registry: nothing deployed (ADR 0024). Shipping faster safely needs a deployment that proves itself on production signals and undoes itself when it does not — plus the release-velocity hygiene the docs claimed but CI never enforced.

## What Changes

- **Canary on Kubernetes (primary)**: `rollout.enabled` swaps the api Deployment for an Argo Rollouts canary (20% → analyze → 50% → analyze → 100%); the pod spec is extracted into a shared Helm helper so Deployment and Rollout cannot drift. `AnalysisTemplate` polls Prometheus for the SLO metrics (availability ≥ 99%, p95 ≤ 500ms; NaN-safe for quiet canaries); first breach aborts back to stable. Canary Service twin; HPA target follows the workload kind.
- **GitOps**: `deploy/gitops/application.yaml` — Argo CD Application pulling the OCI chart with automated sync + selfHeal; `docs/gitops.md` covers promote/rollback flows.
- **Nomad alternative**: `deploy/nomad/go-template.nomad.hcl` with native `canary = 1, auto_promote, auto_revert` and `/health/ready` gates; `make nomad-up/run/stop`. `docs/orchestrators.md` compares K8s/Nomad/Swarm and records why Swarm is documented, not shipped.
- **Local rehearsal**: `scripts/kind-demo.sh` (`make kind-up/kind-release/kind-down`) — kind + Prometheus + Argo Rollouts + the app as a Rollout, including the abort path.
- **Release velocity**: CI `codegen` job runs `make generate && git diff --exit-code` (drift gate the spec text claimed); SLSA build provenance attestation in `release.yml` (closes ADR 0018's next step); release-drafter workflow + config for automatic release notes.

## Capabilities

### Modified Capabilities

- `deployment` (canary, GitOps, provenance, changelog)

## Impact

- Chart renders both modes (verified via helm lint/template with `rollout.enabled` on/off); worker stays a Deployment. Nomad job uses host.docker.internal defaults for the dev-agent path.
