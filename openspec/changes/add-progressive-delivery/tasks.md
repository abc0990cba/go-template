## 1. Rollout
- [x] 1.1 Shared api pod spec helper; Deployment/Rollout switch; canary Service; HPA target follows kind
- [x] 1.2 AnalysisTemplate (SLO metrics, NaN-safe); rollout values (steps, thresholds, prometheus address)
- [x] 1.3 helm lint + template verified in both modes

## 2. GitOps And Alternatives
- [x] 2.1 `deploy/gitops/application.yaml` + `docs/gitops.md`
- [x] 2.2 Nomad job spec + `make nomad-*` targets; `docs/orchestrators.md` (incl. Swarm verdict)
- [x] 2.3 `scripts/kind-demo.sh` walkthrough incl. forced-abort instructions

## 3. Release Velocity
- [x] 3.1 CI codegen drift gate
- [x] 3.2 SLSA provenance attestation in release.yml
- [x] 3.3 release-drafter workflow + config
