# CI/CD reference

Two pipelines share one trigger (a `v*` tag) and one philosophy (build once,
scan, deploy the same immutable tag):

| Pipeline | File | Runs where | Role |
|---|---|---|---|
| GitHub Actions | `.github/workflows/` | GitHub | PR quality gates + the public release artifact train (GHCR) |
| Woodpecker | `.woodpecker/build-deploy.yml` | your VPS | **The primary build+deploy path when self-hosting** — builds and deploys straight into the local Nomad cluster |

Deployment mechanics of where each lands: [kubernetes.md](kubernetes.md)
(GHCR + Helm/Argo CD) and [nomad.md](nomad.md) (Woodpecker/Ansible + Nomad).

## GitHub Actions

### `ci.yml` — PR and main-branch quality gates

Runs on pushes to `main` and on every PR. Concurrency: a new push to the
same PR supersedes its in-flight run; `main` pushes are never cancelled.

A **`changes` job** (`dorny/paths-filter`) computes two filters and gates
the rest so untouched areas skip checks they cannot affect:

- `go`: `**.go`, `go.mod`, `go.sum`, `api/openapi/**`, `queries/**`,
  `migrations/**`, `Makefile`, `.golangci.yml`
- `web`: `web/**`, `api/openapi/**`, `Makefile`

The eight gated jobs:

| Job | Gate | What it does |
|---|---|---|
| `web` | web | oxlint + oxfmt format check, type-check + build, architecture boundaries (`make web-check-boundaries`), size budget (`make check-size-web`: ≤400/file, ≤80/route), vitest with coverage floor |
| `build-test` | go | `go vet`, `go build`, Go size budget (`make check-size`, ≤400 lines per hand-written production file), coverage floor (`make coverage-check`) |
| `lint` | go | golangci-lint **v2.14.0 pinned** to the config format the repo uses |
| `vulncheck` | go | `make vulncheck` — govulncheck against the Go vulnerability DB; fails on call paths into vulnerable symbols |
| `integration` | go | store contract tests: `go test -tags=integration ./test/integration/ -count=1 -timeout 10m` |
| `web-e2e` | web **or** go | Playwright in Chromium over the real binary (SPA built, embedded, in-memory store + demo seed); uploads `web/playwright-report/` on failure |
| `codegen` | **always** | the drift gate: `make generate` (sqlc, oapi-codegen, TS schema) + `make lint-spec` + `git diff --exit-code` — a PR that edits queries or the OpenAPI spec without regenerating fails here |
| `docker` | go **or** web | builds the production image (no push) with `VERSION=ci-<sha>` so Dockerfile regressions surface in the PR |

All actions are SHA-pinned; the golangci-lint version must be ≥ the module's
toolchain directive or older analyzers refuse the module.

### `release.yml` — tag → artifact train

Triggers on tags `v*`. Job `image`:

1. **Multi-arch buildx** (`linux/amd64` + `linux/arm64`) pushed to GHCR as
   `ghcr.io/<org>/go-template` with semver tags `{{version}}` (e.g. `1.2.3`)
   and `{{major}}.{{minor}}` (e.g. `1.2`); GHA cache.
2. **Trivy** scan — fails the build on CRITICAL/HIGH with unfixed CVEs;
   SARIF uploaded to code scanning.
3. **Syft** SPDX SBOM (`sbom.spdx.json` artifact).
4. **cosign keyless** signature via the workflow's OIDC token
   (`id-token: write`).
5. **SLSA build provenance** attestation
   (`actions/attest-build-provenance`, pushed to the registry). Consumers
   verify with `gh attestation verify <artifact> -R <repo>` or
   `cosign verify-attestation --type slsaprovenance`.

Job `chart` (needs `image`): `helm lint` → `helm package` → `helm push` to
`oci://ghcr.io/<org>/charts`. That packaged chart is what Argo CD pulls in
the GitOps flow ([gitops.md](gitops.md)).

### `dast.yml` — weekly OWASP ZAP baseline

Triggers: pushes to `main`, `workflow_dispatch`, and a schedule — **Mondays
04:00 UTC**. Boots the compose stack (the production image, demo dataset
seeded), waits for `/health/ready`, then runs the ZAP baseline scan
(`ghcr.io/zaproxy/zaproxy:stable zap-baseline.py -t http://127.0.0.1:8080`,
120s timeout) with `--network host` so the scanner reaches the published
port. Fails only on FAIL-level findings; the full HTML report is archived as
an artifact either way. Alerts are advisory input — triage, fix, or suppress
with a documented reason. `make dast` runs the identical scan locally.

### `release-drafter.yml` — release notes that write themselves

Runs on pushes to `main` and PR events; `.github/release-drafter.yml` turns
merged PRs into a draft release grouped by category. Categories come from
**PR-title prefixes** (via autolabeler regexes — no conventional-commit
discipline required, just one-line human titles):

| Section | PR title matches |
|---|---|
| ⚠ Breaking changes | `!` anywhere in the title, or "breaking" |
| ✨ Features | `feat…`, `add…`, `support…` |
| 🛡 Reliability & observability | `metric`, `trace`, `monitor`, `grafana`, `prometheus`, `slo`, `alert`, `profil…` / `breaker`, `bulkhead`, `retry`, `chaos`, `retention`, `dlq`, `outbox` |
| 🚀 Release & deployment | `ci…`, `workflow`, `github actions` / `helm`, `k8s`, `kubernetes`, `nomad`, `rollout`, `canary`, `deploy` |
| 🐛 Fixes | `fix…`, `correct…`, `repair…` |
| 📄 Documentation | `docs…`, `doc…`, `readme`, `adr` |

The same labels drive **version resolution**: `breaking` → major,
`feature`/`enhancement` → minor, everything else → patch. PRs labelled
`skip-changelog` are excluded.

## Woodpecker — the primary path on your own VPS

`.woodpecker/build-deploy.yml` fires on `v*` tags and runs *on the VPS*,
against the local Docker socket and the local Nomad API. Five steps:

| Step | Image | What happens |
|---|---|---|
| `build` | `docker:28` | `docker build` with `VERSION=$CI_COMMIT_TAG`, `COMMIT=<sha8>`, `DATE` stamped; tagged `127.0.0.1:5000/go-template:<tag>` and `:<sha8>` |
| `scan (security gate)` | `aquasec/trivy:0.63.0` | `trivy image --exit-code 1 --ignore-unfixed --severity CRITICAL,HIGH` — the pipeline stops here on a bad image |
| `push` | `docker:28` | pushes both tags to the **local registry** (`127.0.0.1:5000`) |
| `deploy` | `hashicorp/nomad:2.0.7` | mTLS to the Nomad API (`NOMAD_ADDR=https://172.17.0.1:4646`, CA + token from Woodpecker secrets), then **`nomad job run -var image_tag=… -var app_domain=… deploy/nomad/app.nomad.hcl`** — the same job spec Ansible uses. The run is **blocking**: it exits non-zero unless the canary passed `/health/ready` and was promoted; `auto_revert` has already rolled back otherwise |
| `smoke` | `curlimages/curl:8.12.1` | polls `https://$APP_DOMAIN/health/ready` up to 30× / 10s, prints `/version`, fails the pipeline if never ready |

The pinned `hashicorp/nomad` image (rather than a downloaded CLI) also keeps
the pipeline working on networks whose egress blocks `releases.hashicorp.com`.

### One-time setup (full detail: [vps/deployment.md](vps/deployment.md) CI setup section)

1. **GitHub OAuth app** — `https://github.com/settings/developers`, homepage
   `https://ci.example.com`, callback
   `https://ci.example.com/oauth2/response`. Put the client id/secret into
   `secrets.yml` (`woodpecker_github_client_id` / `woodpecker_github_client_secret`),
   then re-render the job: `make vps-update JOBS=woodpecker`.
2. Open `https://ci.example.com`, sign in with GitHub (the first user becomes
   admin while `woodpecker_admin_user` is empty), and **activate the
   repository**.
3. Add three **Woodpecker repository secrets**, each enabled for the *tag*
   event (Repository → Secrets):
   - `nomad_token` ← `ssh <vps> sudo cat /etc/nomad.d/tokens/deploy.token`
     (the least-privilege deploy token)
   - `nomad_ca_cert` ← `ssh <vps> sudo cat /etc/nomad.d/tls/ca.pem`
   - `app_domain` ← your `app.example.com`

Private repos: the one-shot bootstrap build clones without credentials —
push a first image from your machine, or add a deploy token to the clone
step. The CI path itself authenticates via the OAuth app, not repo
credentials.

## Release checklist

1. **Versioning**: semver tags `vMAJOR.MINOR.PATCH`. Bump major on breaking
   changes, minor for features, patch otherwise — the same policy the
   release-drafter's autolabeler encodes. Keep PR titles prefix-disciplined
   (table above) so draft notes categorize themselves.
2. **What a tag triggers, per path**:
   - *GitHub*: `release.yml` builds and publishes the multi-arch image +
     chart to GHCR (scan/SBOM/sign/provenance included); release-drafter has
     already drafted the notes on main — review and publish the draft.
   - *Woodpecker (your VPS)*: build → Trivy → local registry → blocking
     Nomad deploy → smoke. If the canary never turns healthy, the pipeline
     fails **and the previous version is already serving** (`auto_revert`).
   - Running both is the intended shape: Woodpecker deploys your VPS;
     the GHCR artifacts are the portable/fallback path (point
     `app_image: ghcr.io/<org>/go-template` at it any time — see
     [vps/deployment.md](vps/deployment.md)).
3. **Verify after deploy**: `make vps-verify` (post-deploy check table) or
   `curl https://$APP_DOMAIN/health/ready` and `/version`;
   `make vps-status` for Nomad job status. If a healthy-but-wrong deploy
   shipped: `make vps-rollback`.
4. **Hotfix**: fix on a branch → PR → merge → tag `vX.Y.Z+1` — the same
   train runs; no special casing.
