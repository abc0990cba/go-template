## 1. Chart
- [x] 1.1 Chart.yaml/values.yaml/_helpers (shared env + secrets `_FILE` wiring)
- [x] 1.2 api Deployment (probes, rolling, securityContext), worker Deployment (metrics), Service, Ingress (optional), HPA, PDB, SA, NOTES
- [x] 1.3 `make helm-lint` / `helm-template`; lint + render verified

## 2. Release Pipeline
- [x] 2.1 `release.yml`: multi-arch GHCR push, Trivy gate (SARIF), Syft SBOM, cosign keyless sign, chart package + OCI push
- [x] 2.2 SHA-pin every action in both workflows; pin golangci-lint to v2.x

## 3. Verification And Docs
- [x] 3.1 helm lint + template render green; workflow YAML parses
- [x] 3.2 docs/deployment.md (chart usage, sizing note, pipeline walkthrough)
