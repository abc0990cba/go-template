# Documentation Map & Reading Guide

This is the **entry point to all documentation** for this repository — a
full-stack application template: a Go backend (Gin + pgx/sqlc/PostgreSQL,
Redis/asynq jobs, Kafka/NATS ingestion, ClickHouse analytics, S3 file
storage, OpenTelemetry/Prometheus observability) that serves an embedded
React 19 + Vite SPA, deployable from a laptop to a VPS or Kubernetes.

Start here if you are asking any of these:

- *"I just cloned this — what do I run first?"* → [Reading order](#reading-order), step 1.
- *"Where is X documented?"* → [Find by task](#find-by-task) or the [folder map](#how-the-documentation-is-organized).
- *"I'm the kind of person who…"* → jump straight to [your track](#tracks-by-role).

---

## How the documentation is organized

Every document belongs to exactly one of seven categories. The category
tells you **when** to read it: guides teach, references answer, runbooks
interrupt, roadmaps point forward.

| Folder | What lives here | When you open it |
|---|---|---|
| [`getting-started/`](getting-started/) | Everything between `git clone` and a running app you can change | First contact, setting up, daily development, when something breaks |
| [`guides/`](guides/) | How each feature works and how to use it (auth, RBAC, tickets, webhooks, streams…) | When you touch or configure a feature |
| [`reference/`](reference/) | Look-up material: every env var, every make target, CLI tools, API contract, repo layout | When you need the exact name, flag, or default |
| [`deployment/`](deployment/) | Running in non-dev environments: Docker, VPS (Ansible+Nomad), Kubernetes/Helm, GitOps, CI/CD | When you ship or operate |
| [`operations/`](operations/) | Living with the system: observability, SLOs, reliability, chaos, alert runbooks | When something is on fire, or before it is |
| [`testing/`](testing/) | Test strategy and the four traceability matrices | When you write or review tests |
| [`architecture/`](architecture/) | The master set: 15 chapters of C4 views, flows, and 26 ADRs — traceable to code | When you need *why*, not just *how* |
| [`roadmap/`](roadmap/) | Where the template is going, and its known limitations & technical debt | When planning work, or wondering "why is this like this?" |

Two more things live under `docs/`: [`architecture/history/`](architecture/history/) holds the
completed (historical) architecture-migration record, and [`screenshots/`](screenshots/) holds the
images used by the root README.

Related, outside `docs/`:

- [`../CONTRIBUTING.md`](../CONTRIBUTING.md) — how to work on this repo: workflow, quality gates, PR conventions.
- [`../web/AGENTS.md`](../web/AGENTS.md) — the frontend architecture contract (layering rules, enforced by CI).
- [`../.env.example`](../.env.example) — every configuration variable, commented; rendered as tables in [`reference/configuration.md`](reference/configuration.md).
- [`../openspec/`](../openspec/) — the change-management archive: every feature that landed has a proposal here. **Historical record** — file paths inside old proposals reflect where docs lived *at that time*, not today.

---

## Reading order

The order below is the one a new person actually needs. Steps 1–3 make the
app run; 4–6 make you productive; 7–9 make you dangerous. Each step says
what you'll walk away with. If you only read three documents, read the
quickstart, the configuration reference, and the deployment hub.

1. **[Quickstart](getting-started/quickstart.md)** *(15 minutes)* — run the
   full stack with `make docker-up` (or zero-dependency with `make run`),
   sign in with the demo accounts, and see every feature working. You'll
   walk away with: the app running locally, a feel for what the product
   does, and the port map.

2. **[Prerequisites](getting-started/prerequisites.md)** *(10 minutes)* —
   the exact toolchain (Go 1.26, Node 22, Docker) and which optional tools
   you need for which task. You'll walk away with: a verified environment,
   and the knowledge that most codegen tools are version-pinned and
   downloaded on demand — you don't install them.

3. **[Development workflow](getting-started/development-workflow.md)** *(30
   minutes)* — the daily loop (`make dev` and what it actually starts), the
   OpenAPI-contract codegen pipeline, how a database migration ships, and
   every quality gate CI will enforce on you. You'll walk away with: your
   first code change passing all local gates. **Pay attention to** the
   codegen section — hand-editing generated files is the classic beginner
   mistake here.

4. **[Configuration reference](reference/configuration.md)** *(reference —
   skim once, return often)* — every environment variable, its default,
   its validation rule, and where each deployment path sets it. **Pay
   attention to** the `_FILE` secret pattern and the degradation ladder
   (which dependencies are optional and what happens without them) — both
   are core design decisions of this template.

5. **The feature you're about to touch** — each one is documented in
   [`guides/`](guides/): authentication & identity ([auth](guides/auth.md),
   [MFA](guides/mfa.md), [token grants](guides/token-auth.md),
   [invitations](guides/invitations.md)), [RBAC](guides/rbac.md),
   [tickets & approvals](guides/tickets.md), [webhooks](guides/webhooks.md),
   [jobs/queues](guides/jobs.md), [file storage](guides/files.md),
   [data export](guides/data-export.md), [caching & search](guides/caching-search.md),
   [analytics](guides/analytics.md), [event streams](guides/enrichment-streams.md),
   [database](guides/database.md), [frontend](guides/frontend.md),
   [application overview](guides/application-overview.md). You'll walk away
   with: the service ports, storage tables, API surface, and admin surface
   of that feature.

6. **[Project structure](reference/project-structure.md)** *(20 minutes)* —
   the repo layout and where code lives, backed by
   [`architecture/04-components-and-code-map.md`](architecture/04-components-and-code-map.md).
   You'll walk away with: the ability to guess any file's location correctly.

7. **[Deployment hub](deployment/README.md)** *(choose your path)* — one
   comparison table of the five run/deploy paths (local compose, VPS via
   Ansible+Nomad, Kubernetes via Helm, GitOps, kind demo), then the page
   for your path. **Pay attention to** the environment matrix — what
   changes between dev, demo, staging, and production. You'll walk away
   with: the thing running somewhere that isn't your laptop.

8. **[Testing overview](testing/README.md)** *(30 minutes)* — the test
   pyramid (unit → testcontainers integration → e2e → k6 → fuzz), the
   store-contract suite that keeps two storage adapters honest, and the
   four matrices that map requirements to evidence. You'll walk away with:
   knowing which test layer your change belongs in.

9. **[Architecture chapters](architecture/README.md) + [ADRs](architecture/decisions/README.md)**
   *(the deep end)* — 15 chapters from system context to code map, each
   opening with a one-sentence thesis; 26 ADRs, each with
   context/decision/consequences. **Pay attention to** ADR 0004 (graceful
   degradation) — it explains most of the template's unusual design
   choices. You'll walk away with: the *why* behind everything.

Optional but recommended early: [Study path](getting-started/study-path.md)
turns steps 1–9 into a structured 2–3 week curriculum with exercises, and
[Troubleshooting](getting-started/troubleshooting.md) solves the problems
you hit along the way.

---

## Tracks by role

Different readers need different subsets. Each track lists documents in
order; numbering refers to the reading order above.

### 🏃 "I just want to run it"
[Quickstart](getting-started/quickstart.md) ·
[Prerequisites](getting-started/prerequisites.md) ·
[Configuration reference](reference/configuration.md) (minimal-dev example) ·
[Troubleshooting](getting-started/troubleshooting.md) when stuck ·
then [Deployment hub](deployment/README.md) if "it" means a server.

### 👨‍💻 Backend developer joining the project
Reading order 1–6, then: [Database guide](guides/database.md) →
[architecture chapters 06–09](architecture/README.md) (data model, auth,
authorization, business domains) → [Testing overview](testing/README.md) →
[Makefile reference](reference/make-targets.md) → skim
[known issues](roadmap/known-issues.md) to learn the sharp edges.
House rule: read [ADR 0001](architecture/decisions/0001-hexagonal-architecture.md)
and [ADR 0021](architecture/decisions/0021-package-by-feature.md) before
creating any new package.

### 🎨 Frontend developer
[Quickstart](getting-started/quickstart.md) (run modes + ports) ·
[Frontend guide](guides/frontend.md) ·
**[`web/AGENTS.md`](../web/AGENTS.md)** — the binding contract: layer
boundaries (`routes → features → shared`), file budgets, the
no-raw-fetch rule · [API contract](reference/api.md) (the TS client is
generated from OpenAPI) · [Development workflow](getting-started/development-workflow.md)
(codegen + quality gates) · [web tests](testing/testing.md) (Vitest + MSW
contract gate + Playwright).

### 🚀 DevOps / SRE
[Deployment hub](deployment/README.md) → your path:
[VPS quickstart](deployment/vps/QUICKSTART.md) (recommended, one command) or
[Kubernetes](deployment/kubernetes.md) →
[CI/CD reference](deployment/ci-cd.md) →
[Configuration reference](reference/configuration.md) (production example, `_FILE` secrets) →
[Observability](operations/observability.md) + [SLOs](operations/slo.md) →
[Alert runbooks](operations/runbooks/alerts.md) →
[VPS runbook](deployment/vps/runbook.md) for day-2 operations ·
[Reliability](operations/reliability.md) and [chaos lab](operations/chaos.md)
when you want to rehearse failure instead of experiencing it.

### 🎓 Student studying production techniques
[Study path](getting-started/study-path.md) — a nine-stage curriculum with
exercises that sequences everything below — then the
[architecture set](architecture/README.md) as your textbook and the
[ADRs](architecture/decisions/README.md) as case studies in engineering
judgment. The [roadmap](roadmap/roadmap.md) doubles as a source of practice
projects: every open item tells you where to start.

---

## Find by task

| I want to… | Go to |
|---|---|
| Run the app the first time | [getting-started/quickstart.md](getting-started/quickstart.md) |
| Develop a feature (daily loop, hot reload) | [getting-started/development-workflow.md](getting-started/development-workflow.md) |
| Add a new feature end-to-end | [architecture/15-template-guide.md](architecture/15-template-guide.md) (recipe) + [development-workflow](getting-started/development-workflow.md) |
| Add / change an environment variable | [reference/configuration.md](reference/configuration.md) (+ update `.env.example` in the same PR) |
| Add a database migration | [guides/database.md](guides/database.md) + the migration recipe in [development-workflow](getting-started/development-workflow.md) |
| Change the API contract | [reference/api.md](reference/api.md) (OpenAPI-first flow) |
| Understand / add a permission | [guides/rbac.md](guides/rbac.md) |
| Wire up OAuth, MFA, or API keys | [guides/auth.md](guides/auth.md), [guides/mfa.md](guides/mfa.md), [guides/token-auth.md](guides/token-auth.md) |
| Work on tickets / approvals | [guides/tickets.md](guides/tickets.md) |
| Send or receive webhooks | [guides/webhooks.md](guides/webhooks.md) |
| Run Kafka/NATS locally, replay the DLQ | [guides/enrichment-streams.md](guides/enrichment-streams.md) |
| Upload / serve files | [guides/files.md](guides/files.md) |
| Run background jobs | [guides/jobs.md](guides/jobs.md) |
| Find a make target or CLI tool | [reference/make-targets.md](reference/make-targets.md), [reference/cli-tools.md](reference/cli-tools.md) |
| Seed demo or perf data | [reference/cli-tools.md](reference/cli-tools.md) (`cmd/seed`, `cmd/seed-demo`) |
| Fix a local dev problem | [getting-started/troubleshooting.md](getting-started/troubleshooting.md) |
| Deploy to a VPS in one command | [deployment/vps/QUICKSTART.md](deployment/vps/QUICKSTART.md) |
| Deploy to Kubernetes | [deployment/kubernetes.md](deployment/kubernetes.md) |
| Understand the CI/CD pipelines | [deployment/ci-cd.md](deployment/ci-cd.md) |
| Operate day-to-day (logs, backups, rollback) | [deployment/vps/runbook.md](deployment/vps/runbook.md) |
| Interpret a Prometheus alert | [operations/runbooks/alerts.md](operations/runbooks/alerts.md) |
| Check the SLOs / burn rates | [operations/slo.md](operations/slo.md) |
| Break things on purpose (chaos) | [operations/chaos.md](operations/chaos.md) |
| Run the performance suite | [testing/performance.md](testing/performance.md) + [performance matrix](testing/performance-matrix.md) |
| Write / place a test | [testing/README.md](testing/README.md) + [testing/testing.md](testing/testing.md) |
| Rebrand the template as my project | [architecture/15-template-guide.md](architecture/15-template-guide.md) |
| Know what's broken or risky | [roadmap/known-issues.md](roadmap/known-issues.md) |
| Know what's next | [roadmap/roadmap.md](roadmap/roadmap.md) |
| Understand *why* a decision was made | [architecture/decisions/](architecture/decisions/README.md) |

---

## Conventions

**Document types.** *Guides* teach a task or feature (narrative + examples).
*References* are lookup tables (complete, precise, no narrative). *Runbooks*
are interrupt-driven checklists (symptom → diagnosis → fix). *ADRs* record a
decision with context and consequences and are **immutable** — to change a
decision, write a new ADR that supersedes the old one. *Matrices* map
requirements to evidence and are updated in the same PR as the change they
describe.

**Adding a document.** Put it in the folder matching its type (table at the
top). If it's a feature guide, also link it from the root README's feature
list and from this page's reading order step 5. One topic per file; if a
file exceeds ~500 lines, split it.

**Keeping reference docs honest.** Three files mirror source of truth and
will fail you silently if you forget them:
- [`reference/configuration.md`](reference/configuration.md) must match `internal/config/` and `.env.example` — change all three together.
- [`reference/make-targets.md`](reference/make-targets.md) must match the `Makefile` — new target, new row.
- The matrices in [`testing/`](testing/README.md) must match reality — flip a cell only when the evidence exists.

**Links.** Use relative links between docs. When you move a file, `git mv`
it and fix references — a quick `grep -rn "old-name.md" --include='*.md' .`
finds them.

**Language.** All documentation is in English. (The product UI itself ships
an en/ru locale switch — that's separate; see [guides/frontend.md](guides/frontend.md).)

**Where truth lives.** Docs describe the code; the code wins. If you find a
doc contradicting the code, fix the doc — and if the *code* is wrong,
[file it in the known-issues registry](roadmap/known-issues.md) or fix it in
the same PR.
