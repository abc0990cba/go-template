# Architecture Documentation — Go Full-Stack Template

This is the master documentation set for the repository: what the system is,
how it is built, why every choice was made, and how to reuse it as a template
for your own full-stack application.

Everything here is written against the actual source code — every diagram,
table and flow can be traced to files, functions and endpoints. When this
document and the code disagree, the code wins and the doc should be fixed.

---

## How this documentation is organized

The set follows **world-standard documentation conventions**:

| Convention | What it is | Where it is used here |
|---|---|---|
| **C4 model** (Simon Brown) | Hierarchical architecture diagrams: Context → Containers → Components → Code | Docs `02`, `03`, `04` |
| **arc42** | The classic architecture-documentation skeleton (intro, constraints, runtime views, deployment, decisions, quality) | The overall ordering of docs `01`–`14` |
| **ADR** (Architecture Decision Records, M. Nygard format) | One short numbered document per significant architectural decision: *Status / Context / Decision / Consequences* | [`decisions/`](decisions/) — ADR 0001–0020 |
| **RFC 9457 problem+json** | The standard error-response format used by the API itself | Doc `04`, `11` |
| **Mermaid** | All diagrams are Mermaid blocks — they render directly on GitHub | everywhere |

## The documents

### Understand the system

| # | Document | Answers the question |
|---|---|---|
| 01 | [Introduction & Principles](01-introduction-and-principles.md) | What is this, for whom, what rules does it follow? |
| 02 | [System Context (C4 L1)](02-system-context.md) | Who and what surrounds the system? |
| 03 | [Containers & Runtime Views (C4 L2)](03-containers-and-runtime-views.md) | What are the deployable pieces and how do they talk? |
| 04 | [Components & Code Map (C4 L3)](04-components-and-code-map.md) | What is inside each binary, layer by layer, request by request? |
| 05 | [Technology Stack](05-technology-stack.md) | Which technology for what, and why that one? |

### Understand the data and the behavior

| # | Document | Answers the question |
|---|---|---|
| 06 | [Data Model](06-data-model.md) | What do we store, where, and with which guarantees? |
| 07 | [Flows: Authentication & Identity](07-flows-authentication-and-identity.md) | How do login, MFA, JWT, OAuth, API keys, sessions work — step by step? |
| 08 | [Flows: Authorization & Tenancy](08-flows-authorization-and-tenancy.md) | How does RBAC, multi-tenancy, invitations, impersonation work? |
| 09 | [Flows: Business Domains](09-flows-business-domains.md) | Tickets, files, exports, webhooks, jobs, notifications, caching, idempotency? |
| 10 | [Flows: Event Streams](10-flows-event-streams.md) | How does exactly-once enrichment over Kafka + NATS work? |

### Operate and extend the system

| # | Document | Answers the question |
|---|---|---|
| 11 | [Security](11-security.md) | What is the security model, what is protected, how? |
| 12 | [Observability & Reliability](12-observability-and-reliability.md) | How do we see inside it and keep it alive? |
| 13 | [Deployment & Operations](13-deployment-and-operations.md) | How is it built, shipped, configured, scaled? |
| 14 | [Testing & Quality](14-testing-and-quality.md) | How is correctness proven at every level? |
| 15 | [Template Guide](15-template-guide.md) | How do I turn this into *my* application and add feature #1? |

### Decisions

[`decisions/`](decisions/) holds 20 ADRs — one per architectural choice that
shapes the whole project (hexagonal architecture, monolith-first, sqlc over
ORM, session cookies vs JWT, exactly-once ingestion, transactional outbox,
distroless supply chain, and more). Start from
[decisions/0001-hexagonal-architecture.md](decisions/0001-hexagonal-architecture.md).

## Reading paths

- **I have 5 minutes** — doc `01` + the Context diagram in `02`.
- **I want to evaluate the stack** — docs `05`, `03`, and the ADR index.
- **I will build a feature on it** — docs `04`, `06`, `15`, then the flows
  chapter matching the feature area.
- **I will run it in production** — docs `13`, `12`, `11`.
- **I need to debug something** — `12` (metrics/health), `04` (request
  lifecycle), then the relevant flows chapter.

## Feature-level documentation

Short, per-feature guides live one level up in [`docs/`](../) — e.g.
[tickets](../tickets.md), [mfa](../mfa.md),
[enrichment-streams](../enrichment-streams.md), [files](../files.md),
[data-export](../data-export.md), [rbac](../rbac.md),
[token-auth](../token-auth.md), [invitations](../invitations.md),
[caching-search](../caching-search.md), [deployment](../deployment.md),
[quickstart](../quickstart.md),
[application-overview](../application-overview.md) (component map with
hands-on verification for every component), and
[regression-checklist](../regression-checklist.md) (the 33-check live
walkthrough). These architecture chapters give the deep, connected picture;
the feature docs give the focused how-to.
