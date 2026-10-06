# Known issues and technical debt

The registry of known limitations, deliberate trade-offs, and rough edges.
"We know, and here is why" beats silence: every entry carries a mandatory
**"Why it is this way"** field — the root cause or the trade-off that made
the current state the right call. An entry is not an invitation to fix
things in drive-by PRs; it is the queue work is chosen from deliberately.

Severity: **high** = architectural limit an adopter must plan around;
**medium** = misleads or costs contributors/adopters in real use; **low** =
cosmetic, documented, or cheap to work around.

## How to work on these

- Every fix graduates through the OpenSpec workflow like any other work:
  open `openspec/changes/<name>/` per
  [the reference](../reference/openspec.md), fix, then update the matrices
  and this entry's **Status**. Trivial doc/Makefile corrections (KI-008,
  KI-014, KI-017) may be a single PR that includes the entry update.
- The candidate backlog lives in [roadmap.md](roadmap.md); several items
  there (devtools target, webhook rotation, matrix cells) exist *because*
  of entries here.
- When an entry is resolved, delete it in the same change — the registry
  must describe the tree as it is, not history.

## Summary

| ID | Area | Severity | Status | One-line summary |
|---|---|---|---|---|
| KI-001 | Deployment (VPS) | high | Open — accepted trade-off | Single-node VPS: no HA, Postgres not replicated |
| KI-002 | Configuration | medium | Open — mitigated | Same knobs defined across five config surfaces; drift risk |
| KI-003 | Testing (E2E) | medium | Open — accepted trade-off | Browser E2E: Chromium-only, 1 worker, in-memory store |
| KI-004 | Testing (coverage) | medium | Open | Coverage floor measures unit runs only; integration unmerged |
| KI-005 | Observability | medium | Open — needs adopter input | Alertmanager receivers are a harmless placeholder |
| KI-006 | Branding | medium | Open — checklist exists | `your-org`/`go-template` placeholders; rename breaks CI if forgotten |
| KI-007 | Deployment (Ansible) | medium | Open — mitigated | Machine-local secrets/state land in the working tree |
| KI-008 | Docs (README) | medium | Open — one paragraph | README still cites the old demo dataset (120 users / 30 days) |
| KI-009 | Demo (kind) | low | Accepted | kind demo runs the in-memory store; no persistence |
| KI-010 | Demo (brokers) | low | Accepted | `demo-up` uses Redpanda, not Apache Kafka proper |
| KI-011 | Supply chain | low | Accepted | One image carries both api and worker binaries |
| KI-012 | Developer experience | low | Accepted | No native Windows dev workflow; WSL2 required |
| KI-013 | Frontend (i18n) | low | Accepted | Two bespoke i18n layers; landing SSR always renders English |
| KI-014 | Build | low | Open | `make devtools` referenced by `.gitignore` but no such target |
| KI-015 | Docs (matrices) | low | Open | Test/traceability matrices carry unresolved `Planned` cells |
| KI-016 | Testing (e2e) | low | Open — needs live re-run | Go e2e approvals flow recorded as failing pre-migration |
| KI-017 | Docs (deployment) | low | Open | orchestrators.md points at the local Nomad job only |

---

## KI-001 — Single-node VPS: no high availability

- **Area:** Deployment (Ansible + Nomad VPS path)
- **Severity:** high
- **Status:** Open — accepted trade-off
- **Description:** The VPS path puts Nomad server, Nomad client, Postgres, Redis, ClickHouse, brokers, and the observability stack on one host. The job has `canary`/`auto_revert` (health-gated rollouts), but the node is a SPOF and Postgres is not replicated. `docs/deployment/nomad.md` says of `api_replicas`: "multi-node scaling is a Phase-2 change"; the runbook speaks of the "single-node blast radius".
- **Why it is this way:** Deliberate. The VPS path's value is *one command, one host, real TLS, real canary* — a template a developer can afford to run. Replication and multi-node scheduling would triple the Ansible surface and the monthly cost, and the scale-out path (Helm/K8s) already ships for when that matters.
- **Impact:** Reboots and host maintenance are downtime; a disk/CPU ceiling is the scale limit; RPO/RTO are whatever `make vps-backup` / `vps-restore` provide, not zero.
- **Workaround:** Maintenance windows; nightly `vps-backup` with a practiced `vps-restore`; move to the Kubernetes path (HPA, PDB, multi-node) when the workload outgrows one box.
- **Remediation sketch:** Roadmap "Next" — a documented 3-server/2-client Nomad rehearsal, Postgres streaming replication with a failover runbook, stated RTO/RPO — without taxing the single-node default.

## KI-002 — Configuration duplication across five surfaces

- **Area:** Configuration
- **Severity:** medium
- **Status:** Open — mitigated
- **Description:** The same knobs are defined independently in `.env.example` (~106 variables), `docker-compose.yml`, `deploy/ansible/inventories/*/group_vars/vps/main.yml` (+ vaulted `secrets.yml`), `deploy/helm/go-template/values.yaml`, and `deploy/nomad/app.nomad.hcl`. Nothing mechanically keeps the five in step (e.g. the worker metrics port is `:9090` in compose/Nomad, `:9091` in Helm).
- **Why it is this way:** Single-sourcing would require a templating layer over all five surfaces — but Helm and Ansible already template with their own engines, and compose/Nomad consume literals; a sixth meta-layer would add more drift surface than it removes. The accepted mitigation is discipline plus the "Who sets what where" matrix in [configuration.md](../reference/configuration.md).
- **Impact:** A knob changed in one surface silently diverges; adopters copying values between paths can pick up stale defaults.
- **Workaround:** Treat the configuration matrix as the source of truth when touching any surface; identical `APP_*` names make greps reliable.
- **Remediation sketch:** A `cmd/devtools` conformance check that parses all five surfaces and fails on a knob undocumented in the matrix.

## KI-003 — Browser E2E: Chromium-only, one worker, in-memory store

- **Area:** Testing (Playwright suite)
- **Severity:** medium
- **Status:** Open — accepted trade-off
- **Description:** `web/playwright.config.ts` runs a single `chromium` project with `workers: 1`, and the managed webServer starts the Go binary with `APP_DATABASE_URL` empty — i.e. the **in-memory store**. The visual specs additionally self-skip on non-Linux (`test.skip(process.platform !== "linux")`): baselines are Linux-Chromium PNGs.
- **Why it is this way:** Speed and stability. One worker keeps specs sharing one seeded server deterministic; Chromium-only avoids triple-flake CI; the in-memory server makes a browser run one command with no containers. Memory ≡ Postgres is proven separately by the store-contract suite, and the Go e2e suite (`test/e2e`) *does* run against the real docker-compose stack — Postgres-backed journeys are covered, just not through a browser.
- **Impact:** WebKit/Firefox-only regressions pass CI; browser journeys never exercise Postgres (migrations, transaction behavior); a serial suite gets slower as specs grow; visual baselines cannot run on macOS/Windows machines.
- **Workaround:** `E2E_BASE_URL` points the suite at any running stack (`make docker-up`), including a Postgres-backed one.
- **Remediation sketch:** Roadmap "Now" — Firefox/WebKit CI projects; optionally a nightly Postgres-backed e2e job via `E2E_BASE_URL`.

## KI-004 — Coverage floor measures only the unit run

- **Area:** Testing (Go coverage)
- **Severity:** medium
- **Status:** Open
- **Description:** `make coverage-check` enforces a 46% floor (`COVERAGE_FLOOR ?= 46`) over `go test -coverpkg=./internal/... ./internal/... ./cmd/...` only. The integration suites (`test/integration`, incl. the store-contract tests, `-tags=integration`) and e2e run in separate binaries whose profiles are never merged into that number.
- **Why it is this way:** Merging coverage across build-tag-separated binaries needs extra tooling (profile merge or per-binary `-coverpkg`) for a metric that is a *ratchet against regressions*, not a quality claim. The store-contract suite is leaned on as the real safety net; the floor catches rot in the always-on unit run.
- **Impact:** The floor understates real exercised coverage; it cannot catch a regression only integration runs would reveal.
- **Workaround:** None needed — treat 46% as a lower bound by design. The web side already ratchets higher floors (62/54/54/63 per module).
- **Remediation sketch:** Merge profiles in CI (per-tag `-coverprofile` + merge, or `gocovmerge`), re-base the floor honestly, then ratchet.

## KI-005 — Alertmanager default receiver is a placeholder

- **Area:** Observability (alerting)
- **Severity:** medium
- **Status:** Open — needs adopter input
- **Description:** Both receivers in `deploy/alertmanager/alertmanager.yml` (`local-sink`, `critical-sink`) POST to `http://api:8080/metrics`, where the 404s are harmless by design. Firing alerts are visible in the UI (:9093) and Prometheus but are not *delivered* anywhere until an adopter edits the file.
- **Why it is this way:** There is no generic way to pick a real receiver (Slack, PagerDuty, email) in a template — every choice needs adopter secrets and an account. The placeholder keeps the config valid and the UI path demonstrable; the real integrations are pre-written as commented recipes in the same file.
- **Impact:** Out of the box nobody gets paged; an adopter who enables the monitoring profile and walks away has dashboards but no delivery.
- **Workaround:** Uncomment one of the recipes in `deploy/alertmanager/alertmanager.yml`; the compose profile mounts the directory already.
- **Remediation sketch:** An "adopt the alerting stack" step in `docs/operations/observability.md`; optionally a Helm/Ansible variable that drops in a `slack_api_url_file` when provided.

## KI-006 — Placeholder branding throughout

- **Area:** Template identity
- **Severity:** medium
- **Status:** Open — checklist exists
- **Description:** The module path (`github.com/your-org/go-template`), GHCR image references (`ghcr.io/your-org/go-template` in Helm values and the release workflow), chart name, Nomad job name, Ansible vars, and docs all carry the placeholder identity.
- **Why it is this way:** It is a template; the placeholders are the product. The rebrand is documented as a 30-minute, four-identity-string checklist: §15.1 of [the template guide](../architecture/15-template-guide.md).
- **Impact:** An adopter who forks but skips the rename gets CI image pushes against an org they cannot write to.
- **Workaround:** Follow the template guide checklist before the first commit to the fork.
- **Remediation sketch:** A `cmd/devtools rebrand` command performing the mechanical rename plus a grep gate proving no `your-org` remains; a CI assertion could then fail early on un-renamed forks.

## KI-007 — Machine-local Ansible state in the working tree

- **Area:** Deployment (Ansible)
- **Severity:** medium
- **Status:** Open — mitigated
- **Description:** The VPS workflow writes machine-local artifacts next to the code: `deploy/ansible/.vault-pass`, `deploy/ansible/.vps-state/`, and `deploy/ansible/inventories/<site>/group_vars/vps/secrets.yml` (Ansible-Vault-encrypted). All three are gitignored (`.gitignore` lines 48–52) but live inside the repository tree; a real vault file currently sits in this working tree's local inventory.
- **Why it is this way:** Discoverability and tooling simplicity: Ansible resolves group vars relative to the inventory, and the scripts (`deploy/ansible/scripts/vps-local.sh`) manage teardown as a simple `rm -rf deploy/ansible/.vps-state`. Relocating state outside the tree needs an env-var indirection in every script for little gain — the gitignore is the actual safety boundary.
- **Impact:** Sharing, archiving, or containerizing the working directory can leak the vault password and secrets if the ignore rules are bypassed; a careless `git add -f` is unrecoverable.
- **Workaround:** Never commit or share these files; teardown deletes them (the `vps-local.sh` header documents the exact paths).
- **Remediation sketch:** A `GO_TEMPLATE_VPS_STATE_DIR` override (default stays in-tree); a preflight warning in `vps-setup.sh` for world-readable or stale vault files.

## KI-008 — README still cites the old demo dataset

- **Area:** Docs (root README)
- **Severity:** medium
- **Status:** Open — one paragraph
- **Description:** The README features bullet is correct (500 users, 3 orgs, 120 tickets, 90 days of events), but the `APP_DEMO_SEED` paragraph still says "120 users (ten of them login-able …) and 30 days of event history". The code seeds `demoUserCount = 500`, `demoLoginable = 10`, `demoEventDays = 90` (`internal/bootstrap/demodata.go`).
- **Why it is this way:** The dataset was scaled up (10× users, 3× history) when the analytics charts needed believable volume; the features bullet was updated in that pass and this paragraph was missed — the same fact lives in two doc places and only one got the memo.
- **Impact:** New adopters calibrating expectations (or reporting "missing" data) work from wrong numbers.
- **Workaround:** Trust the code and the features bullet; `internal/bootstrap/demodata*.go` is the truth.
- **Remediation sketch:** Fix the paragraph (one line); longer term keep the dataset size in exactly one doc place and link it from the quickstart.

## KI-009 — kind demo runs the in-memory store

- **Area:** Demo (delivery rehearsal)
- **Severity:** low
- **Status:** Accepted
- **Description:** `make kind-up` (`scripts/kind-demo.sh`) deploys the app with empty database/Redis URLs — the in-memory store; pod restarts lose all data.
- **Why it is this way:** The demo targets delivery mechanics — Argo Rollouts canary steps, the SLO analysis gate, the abort path — not storage. Postgres-in-kind (operators, stateful manifests) would roughly triple bootstrap time and add a failure mode to a demo whose point is the rollout.
- **Impact:** Anyone using kind to evaluate persistence will be disappointed; restart-based experiments lose data.
- **Workaround:** None needed for the intended purpose; use `make docker-up` or the Helm chart against a real cluster for storage-inclusive trials.
- **Remediation sketch:** An optional `--with-postgres` flag or variant manifest if demand appears; otherwise say so louder in the script header (it already notes the in-memory store).

## KI-010 — `demo-up` uses Redpanda, not Apache Kafka

- **Area:** Demo (brokers)
- **Severity:** low
- **Status:** Accepted
- **Description:** The streams demo stack (`make demo-up`, `docker-compose.brokers.yml`) brings up Redpanda as the Kafka-API implementation, alongside NATS.
- **Why it is this way:** Single binary, seconds-fast startup, compose-friendly resource use; the app speaks the Kafka protocol via franz-go, and the `add-streaming-enrichment` proposal chose Redpanda deliberately for the local loop.
- **Impact:** Exotic Kafka features (tiered storage, protocol-version edge cases, Kafka-Connect ecosystems) may differ from Apache Kafka proper.
- **Workaround:** The overlay is swappable — point `APP_KAFKA_BROKERS` at a real Kafka container or replace the Redpanda service; the producer/consumer code is implementation-agnostic.
- **Remediation sketch:** Document the swap in the streams runbook; optionally add a `kafka` profile if adopters ask.

## KI-011 — One image, two binaries

- **Area:** Supply chain / packaging
- **Severity:** low
- **Status:** Accepted
- **Description:** The Dockerfile's distroless runtime stage copies both `/app/api` and `/app/worker`; deployments choose per container via the command. Every node pulls both binaries.
- **Why it is this way:** One image means one build, one SBOM, one cosign attestation, one Trivy scan, one tag stream — the supply chain (ADR 0018) stays single-lane and the chart/Nomad job stay trivial. Go static binaries make the duplication cost a few dozen MB.
- **Impact:** Marginally larger images and pulls than per-binary images; a worker-only host stores the api binary too.
- **Workaround:** None needed at template scale.
- **Remediation sketch:** If adopters care: two final stages plus two image tags, with the chart/release workflow learning the second name — an M-sized change against a real supply-chain simplification, so evidence of demand first.

## KI-012 — No native Windows development workflow

- **Area:** Developer experience
- **Severity:** low
- **Status:** Accepted
- **Description:** Every developer-facing script (`scripts/dev.sh`, the `vps-*.sh` family, Ansible, the Makefile's shell assumptions) targets POSIX shells. Windows development requires WSL2, with the repo inside the WSL filesystem.
- **Why it is this way:** The toolchain (bash, Ansible control node, compose semantics, file-watching performance) is POSIX-first; `docs/getting-started/prerequisites.md` states the WSL2 requirement and the `/mnt/c` pitfall explicitly. Native Windows support would fork or rewrite every script for a small share of Go contributors.
- **Impact:** Windows contributors need WSL2 setup knowledge; native PowerShell users are unsupported.
- **Workaround:** WSL2 (documented, including the clone-inside-the-WSL-filesystem rule and troubleshooting notes).
- **Remediation sketch:** None planned; revisit only with sustained demand (roadmap "Later" holds the dev-container escape hatch).

## KI-013 — Two bespoke i18n layers; landing SSR pinned to English

- **Area:** Frontend
- **Severity:** low
- **Status:** Accepted
- **Description:** The SPA has its own flat-dictionary layer (`web/src/shared/lib/i18n.tsx`, en/ru) and the landing a second one (`web/landing/i18n.tsx`, en/ru/fr/de). The landing's server render is always English — the provider mounts as "en" and adopts the visitor's language only after hydration — so crawlers see English.
- **Why it is this way:** Avoiding i18next kept the bundle small and the template dependency-light; the SPA module's own comment names the exit condition ("swap in i18next if keys start proliferating"). The SSR pin is deliberate: the landing is prerendered once at build, and mounting as "en" keeps hydration byte-identical and the crawlable HTML stable.
- **Impact:** Adding a locale is manual in two places; no pluralization/formatting tooling; localized landing pages are invisible to crawlers; two APIs to learn.
- **Workaround:** The i18n parity test (WEB-I18N row in the [test matrix](../testing/test-matrix.md)) fails on missing keys, so locales cannot silently rot; humans get their language post-hydration.
- **Remediation sketch:** Adopt i18next when the dictionary grows (documented escape hatch); per-locale prerender only if non-English SEO becomes a requirement.

## KI-014 — `make devtools` does not exist

- **Area:** Build (Makefile)
- **Severity:** low
- **Status:** Open
- **Description:** `.gitignore` comments "make devtools builds it locally" for `/devtools`, but the Makefile has no `devtools` target; the supported invocation is `DEVTOOLS = go run ./cmd/devtools` or a manual `go build -o devtools ./cmd/devtools`.
- **Why it is this way:** A target lost in a cleanup pass — the tool (`cmd/devtools`, documented in [cli-tools.md](../reference/cli-tools.md)) is invoked via `go run` from the demo targets, and the standalone-build target was dropped without updating the ignore-file comment.
- **Impact:** A misleading comment; contributors following it hit "No rule to make target 'devtools'".
- **Workaround:** `go build -o devtools ./cmd/devtools`, or use the `make demo-*` targets, which `go run` it for you.
- **Remediation sketch:** Add the target (roadmap "Now", S) and keep the comment truthful; one-line fix.

## KI-015 — Unresolved `Planned` cells in the testing matrices

- **Area:** Docs (testing matrices)
- **Severity:** low
- **Status:** Open
- **Description:** The [test matrix](../testing/test-matrix.md) still marks real gaps `Planned` (JOB-001 unit test, AUTH-003 real-OAuth e2e, USER-002 pagination e2e, WEB-GUARDS and WEB-PROFILE e2e), and the Performance column of the [traceability matrix](../testing/traceability-matrix.md) is largely `Planned` even where the scenarios exist.
- **Why it is this way:** The matrices are honest bookkeeping (ADR 0019): cells name what is not yet written rather than being padded with `n/a`. Recent passes chose new features over closing old cells.
- **Impact:** Readers cannot distinguish "not written yet" from "written, matrix stale"; the Performance column undersells the implemented harness.
- **Workaround:** Cross-check the implementation column and actual test files; performance reality lives in [performance-matrix.md](../testing/performance-matrix.md) (all scenarios Implemented).
- **Remediation sketch:** Roadmap "Now" — implement or close every `Planned` cell with a one-line reason in a single OpenSpec change.

## KI-016 — Go e2e approvals flow recorded as failing pre-migration

- **Area:** Testing (Go e2e)
- **Severity:** low
- **Status:** Open — needs live re-run
- **Description:** The migration record ([18](../architecture/history/18-migration-roadmap.md)) documents `TestTicketApprovalFlow` (`test/e2e/happy_path_test.go`) as failing on a HEAD worktree *before* the refactor — "the approvals-inbox premise of the test, not the code". It has not been re-verified against a live stack in this pass (it needs `make docker-up` and demo credentials, and skips itself otherwise).
- **Why it is this way:** The suite is built to skip, not fail, when no stack is reachable — which also lets a stale premise hide; the migration correctly declined to change behavior it had promised to keep byte-identical.
- **Impact:** If the premise is still stale, one e2e journey (submit → approvals inbox → approve) documents the wrong contract.
- **Workaround:** The ticket suites that run in every CI pass (`tickets_test.go`, store contract, web integration tests) cover the approval chain.
- **Remediation sketch:** Run the suite against `make docker-up`; fix the test's inbox premise or the code; then delete this entry.

## KI-017 — orchestrators.md points at the local Nomad job only

- **Area:** Docs (deployment)
- **Severity:** low
- **Status:** Open
- **Description:** The comparison table in [orchestrators.md](../deployment/orchestrators.md) lists `deploy/nomad/go-template.nomad.hcl` as *the* Nomad artifact, while the production single source of truth is `deploy/nomad/app.nomad.hcl` (consumed unchanged by the Ansible VPS path and the Woodpecker pipeline). `docs/deployment/nomad.md` documents the split correctly: one local teaching job, one production job.
- **Why it is this way:** orchestrators.md was written when the local walkthrough job was the only Nomad artifact; the production job arrived with the VPS deploy docs and this page was not revisited.
- **Impact:** A reader comparing orchestrators lands on the demo job and may miss that the production path exists with the same canary/auto-revert semantics.
- **Workaround:** [nomad.md](../deployment/nomad.md) holds the accurate two-file table.
- **Remediation sketch:** One-line doc fix referencing both files (roll into the next docs pass or the KI-015 matrices change).
