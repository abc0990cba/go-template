# GitOps: the handoff from release pipeline to cluster

The release pipeline (`.github/workflows/release.yml`) ends at the
registry: a signed, scanned, SBOM'd multi-arch image and a packaged Helm
chart on GHCR. GitOps closes the loop — the cluster pulls instead of a
human (or a CI job) pushing.

## The pieces

```
tag v1.x.y ──> release.yml ──> ghcr.io/<org>/go-template:<ver>  (image, signed + attested)
                            └─> ghcr.io/<org>/charts/go-template (chart)
                                        │
                             Argo CD polls the repo
                                        ▼
                     deploy/gitops/application.yaml
                     (chart ref + values + sync policy)
                                        ▼
                 Rollout canary: 20% → analysis → 50% → analysis → 100%
                 (AnalysisTemplate polls Prometheus; breach ⇒ auto-abort)
```

- [`deploy/gitops/application.yaml`](../../deploy/gitops/application.yaml) —
  an Argo CD `Application` pointing at the OCI chart, with `rollout.enabled:
  true` and the tracing/sampling values for a staging shape. `automated.syncPolicy`
  with `selfHeal: true` means manual cluster edits are reverted — the repo
  is the single source of truth.
- The canary machinery itself is documented in
  [ADR 0024](../architecture/decisions/0024-progressive-delivery.md); the
  values (steps, thresholds, Prometheus address) live at the bottom of
  [`values.yaml`](../../deploy/helm/go-template/values.yaml).

## Operating it

```sh
# Install Argo CD once (real cluster; not part of the kind demo).
kubectl create namespace argocd
kubectl -n argocd apply -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Hand it the Application: one command, and everything after is git.
kubectl -n argocd apply -f deploy/gitops/application.yaml
```

- **Promote:** bump `targetRevision` (or merge the new tag into the
  values) in Git. Argo CD syncs; the Rollout canaries the change.
- **Roll back:** `git revert` the promotion commit. Argo CD applies the
  old chart; the Rollout treats it as a new revision and canaries *that*
  (instant rollback without canary is `kubectl argo rollouts undo`).
- **Local rehearsal without Argo CD:** `make kind-up && make kind-release`
  runs the same chart + Rollout on kind and shows the abort path.

## Why the deploy step belongs in Git, not in CI

- **Auditability:** the cluster state at any commit is reconstructible;
  "who changed prod" is `git log`, not CI logs.
- **Blast-radius control:** CI credentials never hold cluster admin; the
  cluster only talks to the git/registry read path.
- **Drift correction:** `selfHeal` undoes manual `kubectl edit`s — the
  exact class of change that makes "but it works in staging" lies.
- **Deployment is not the risky part anymore:** the canary gate is. GitOps
  makes deployments frequent and boring, which is what lets the SLO
  budget in `docs/operations/slo.md` fund actual feature work.
