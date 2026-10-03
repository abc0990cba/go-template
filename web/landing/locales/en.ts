import type { FeatureId } from "../feature-ids";

/**
 * The English dictionary — the schema every other locale must satisfy
 * (type Dictionary = typeof en). The EN copy is also what the build-time
 * prerender bakes into the served HTML, so it is what crawlers see.
 */
export const en = {
  meta: {
    title: "Go Template — The Go + React Platform That Ships as One Binary",
  },
  nav: {
    features: "Features",
    how: "How it works",
    faq: "FAQ",
    signIn: "Sign in",
    getStarted: "Get started",
  },
  lang: {
    label: "Language",
  },
  hero: {
    badge: "Open-source starter platform",
    titlePre: "The Go + React platform that ships as",
    titleAccent: "one binary",
    // German needs a suffix after the accent ("…als ein Binary ausgeliefert
    // wird"); EN/RU/FR leave it empty.
    titlePost: "",
    subtitle:
      "Authentication, RBAC, ticketing with approvals, background jobs, event streams, and analytics — a production-shape foundation you can clone, extend, and deploy in minutes.",
    ctaPrimary: "Create a free account",
    ctaSecondary: "Explore the demo",
    checks: ["One container, one port", "No CORS, no separate hosting", "Health probes included"],
  },
  stats: [
    { value: "1", label: "binary to deploy — frontend embedded" },
    { value: "18", label: "feature areas out of the box" },
    { value: "26", label: "architecture decisions recorded" },
    { value: "50+", label: "pages of documentation" },
  ],
  features: {
    title: "Every feature a real product needs",
    subtitle:
      "Eighteen finished feature areas, not a hello-world. Click any card to see exactly what is inside — the plumbing is done, so your first commit is a feature.",
    more: "See what's inside",
    dialogClose: "Close",
    whatsInside: "What's inside",
    items: {
      auth: {
        title: "Authentication & MFA",
        teaser:
          "Cookie sessions, OAuth identities, TOTP two-factor with recovery codes, email verification — wired end to end.",
        description:
          "Everything a real login flow needs, finished on day one: cookie sessions with sliding expiry, OAuth identity linking, TOTP enrollment via QR code, single-use hashed recovery codes, and an mfa.required policy flag that forces two-factor for a user or a whole organization.",
        bullets: [
          "Cookie sessions with rotation and revocation",
          "TOTP enrollment with QR codes and recovery codes",
          "Email verification and password reset with single-use tokens",
          "OAuth identity linking and an enforced MFA policy",
        ],
      },
      rbac: {
        title: "RBAC & policies",
        teaser:
          "Casbin roles with inheritance, a resource:action permission matrix, and a live policy editor.",
        description:
          "Role-based access control on Casbin: role inheritance, multi-tenancy via domains, and a resource:action permission matrix you can edit live in the admin console. Every request is re-checked server-side, and every policy mutation lands in the audit log.",
        bullets: [
          "Roles with inheritance and per-organization grants",
          "Resource:action permission matrix with a raw policy editor",
          "Admin impersonation with a visible banner and audit trail",
          "Permission lookups cached and invalidated on policy changes",
        ],
      },
      orgs: {
        title: "Organizations & invitations",
        teaser:
          "First-class tenants with members, org-scoped API access, and one-step invitation acceptance.",
        description:
          "Casbin domains make organizations real tenants: members, tenant-scoped roles, and org-scoped API access through the X-Org-Id header. Email invitations carry single-use tokens and grant membership plus the domain role in a single step.",
        bullets: [
          "Multi-tenancy via Casbin domains with a scope switcher",
          "Org members with per-tenant role grants",
          "Email invitations with single-use tokens",
          "One-step acceptance: membership and role in one transaction",
        ],
      },
      tickets: {
        title: "Tickets & approvals",
        teaser:
          "A Jira-shaped request workflow: state machine, multi-step approval chains, requester and approver views.",
        description:
          "A production-shaped request workflow: a ticket state machine with distinct “withdrawn by requester” and “rejected by approver” outcomes, configurable multi-step approval chains per ticket type, and separate requester (“My Tickets”) and approver (“Approvals”) workspaces.",
        bullets: [
          "State machine with rework and result delivery",
          "Configurable multi-step approval chains per ticket type",
          "Role-guarded approve and reject decisions",
          "Separate requester and approver interfaces",
        ],
      },
      audit: {
        title: "Audit log",
        teaser: "Immutable who/what/when records with before/after diffs, IP, and request id.",
        description:
          "Every meaningful mutation writes an immutable audit record: actor, action, before/after diff, IP address, and request id — written in the same transaction as the change, with a filterable admin view on top.",
        bullets: [
          "Before/after JSON diffs for every change",
          "Actor, IP, and request-id correlation",
          "Filterable admin console view",
          "Same-transaction writes — no missed entries",
        ],
      },
      settings: {
        title: "Settings & feature flags",
        teaser:
          "Typed key-value settings in the database, a settings UI, public flags, and read-through caching.",
        description:
          "A typed settings store in PostgreSQL with a UI to change it: per-key validation, public flags readable before login, and a read-through cache so hot settings never touch the database on every request.",
        bullets: [
          "Typed keys with validation",
          "Public flags for pre-login pages",
          "Admin settings UI",
          "Read-through caching with invalidation",
        ],
      },
      jobs: {
        title: "Background jobs",
        teaser: "Redis-backed queue (asynq) with retries, scheduling, and a live jobs console.",
        description:
          "A real worker process on asynq and Redis: idempotent enqueueing, retries with backoff, scheduled and periodic tasks, dead-letter handling, and a jobs page with live status updates over Server-Sent Events.",
        bullets: [
          "Separate worker process with idempotent enqueue",
          "Retries with backoff, scheduled and periodic tasks",
          "Live queue console with SSE updates",
          "Graceful shutdown with context propagation",
        ],
      },
      streams: {
        title: "Event streams & outbox",
        teaser:
          "Kafka and NATS JetStream consumers with exactly-once application and a DLQ replay console.",
        description:
          "User enrichment merged from two external systems over Kafka and NATS JetStream: a transactional outbox relay, inbox dedupe, per-source sequence guards, and row-locked field ownership — exactly-once, with a dead-letter queue and an admin replay console.",
        bullets: [
          "Transactional outbox → relay → broker",
          "Inbox dedupe and sequence guards (exactly-once)",
          "Dead-letter queue with admin replay",
          "Kafka and NATS JetStream adapters",
        ],
      },
      webhooks: {
        title: "Webhooks",
        teaser:
          "Event subscriptions with HMAC-SHA256 signatures, queue-backed retries, and a delivery log.",
        description:
          "Subscribe to domain events and deliver them anywhere: HMAC-SHA256-signed payloads, queue-backed delivery with retries and backoff, a full delivery log with response codes, and a test-send button per subscription.",
        bullets: [
          "HMAC-SHA256 signatures on every delivery",
          "Queue-backed retries with backoff",
          "Delivery log with response codes",
          "One-click test send",
        ],
      },
      notifications: {
        title: "Notifications",
        teaser: "In-app notification center with an unread badge and live SSE toasts.",
        description:
          "An in-app notification center with an unread badge, mark-read flows, and live toasts pushed over Server-Sent Events — no polling and no infrastructure beyond the binary.",
        bullets: [
          "Notification center with unread badge",
          "Live toasts over Server-Sent Events",
          "Mark-read and mark-all flows",
          "Per-user notification preferences",
        ],
      },
      apikeys: {
        title: "API keys & JWT grants",
        teaser:
          "gtpl_ bearer tokens with scope narrowing, plus OAuth-style JWT grants with rotation.",
        description:
          "Two first-class token systems: personal API keys (gtpl_…) shown once, hashed at rest, and revocable, with obj:act scope narrowing — and OAuth-style password, MFA, and refresh grants with rotation and reuse detection for mobile and API clients.",
        bullets: [
          "API keys shown once, hashed at rest, revocable",
          "Scope narrowing (obj:act) per key",
          "JWT refresh rotation with reuse detection",
          "Password, MFA, and refresh grants",
        ],
      },
      files: {
        title: "File storage",
        teaser: "S3/MinIO blobs with checksums, permission-scoped downloads, and presigned URLs.",
        description:
          "Checksummed uploads to S3 or MinIO (in-memory for development), permission-scoped downloads, ticket attachments, and presigned URLs — with retention pruning wired into the worker.",
        bullets: [
          "S3, MinIO, and in-memory adapters",
          "Checksum verification on every blob",
          "Permission-scoped downloads",
          "Presigned URLs for direct access",
        ],
      },
      export: {
        title: "GDPR data export",
        teaser:
          "A queued job builds a ZIP of per-entity JSON, owner-only downloads, 7-day retention.",
        description:
          "A GDPR-shaped data export: request it from the profile page, a queued job builds a ZIP of per-entity JSON (profile, tickets, audit, files), download links are owner-only and expire, and old archives are pruned automatically.",
        bullets: [
          "One-click request from the profile page",
          "ZIP of per-entity JSON built by the worker",
          "Owner-only, expiring download links",
          "Automatic 7-day retention pruning",
        ],
      },
      search: {
        title: "Caching & search",
        teaser:
          "Read-through cache with singleflight, ETag/304 on heavy lists, permission-scoped full-text search.",
        description:
          "Hot reads served from a memory or Redis cache with singleflight so a hot-key miss cannot stampede the store, ETag revalidation on heavy lists, and full-text search over users and tickets — permission-scoped and ranked.",
        bullets: [
          "Read-through cache (memory/Redis) with singleflight",
          "ETag / If-None-Match 304 revalidation",
          "Postgres tsvector search with ranking",
          "Results scoped by the caller's permissions",
        ],
      },
      resilience: {
        title: "Resilience toolkit",
        teaser:
          "Circuit breakers, distributed rate limiting, idempotency keys, and bulkheads — all with metrics.",
        description:
          "The failure modes are handled before you ship: circuit breakers around every outbound call, Redis-backed distributed rate limiting with an in-memory fallback, Idempotency-Key middleware that turns client retries into replays, and per-dependency bulkheads — exported as Prometheus metrics.",
        bullets: [
          "Circuit breakers with state metrics",
          "Distributed rate limiting with in-memory fallback",
          "Idempotency-Key replay middleware",
          "Bulkheads with bounded concurrency",
        ],
      },
      observability: {
        title: "Observability & SLOs",
        teaser:
          "OpenTelemetry traces, RED metrics, SLO burn-rate alerts, and dashboards shipped as code.",
        description:
          "Logs, metrics, and traces with trace↔log correlation across every boundary — HTTP, jobs, Kafka/NATS, pgx, Redis, ClickHouse — plus Prometheus RED metrics, continuous profiling, and dashboards, alert rules, and SLO burn-rate alerts shipped as code.",
        bullets: [
          "OpenTelemetry traces across every boundary",
          "Prometheus RED metrics and pool stats",
          "Dashboards and alert rules as code",
          "SLO burn-rate alerting and continuous profiling",
        ],
      },
      deploy: {
        title: "Deploy anywhere",
        teaser: "One binary — or Helm with an SLO-gated canary, GitOps manifests, or Nomad.",
        description:
          "Ship it your way: a single distroless binary with the frontend embedded; a Helm chart with HPA, PDB, probes, and secrets mounts; an SLO-gated canary via Argo Rollouts with auto-rollback; GitOps manifests; or a Nomad job. Releases are signed, scanned, and SBOM-attested.",
        bullets: [
          "One static binary, frontend included",
          "Helm chart with an SLO-gated canary",
          "GitOps manifests and a Nomad alternative",
          "Signed, scanned, SBOM-attested releases",
        ],
      },
      dx: {
        title: "Developer experience",
        teaser:
          "One-command demos, a rich seeded dataset, a regression walkthrough, and OpenAPI codegen.",
        description:
          "The template is built to be explored: make demo-up brings up the brokers, devtools publishes real events, the seeded dataset fills every screen on first launch, a regression script drives every feature end to end, and the frontend API client is generated from the OpenAPI spec.",
        bullets: [
          "make demo-* scenarios: enrichment, DLQ, breakers…",
          "Rich seeded demo data on first launch",
          "Regression walkthrough script across every feature",
          "OpenAPI → TypeScript client codegen",
        ],
      },
    } satisfies Record<
      FeatureId,
      { title: string; teaser: string; description: string; bullets: string[] }
    >,
  },
  how: {
    title: "From clone to production in three steps",
    subtitle: "No wiring weeks. The boring parts are already finished and tested.",
    steps: [
      {
        title: "Clone and configure",
        text: "Environment-driven config, docker compose for Postgres and friends, and a seeded demo dataset to explore on first launch.",
      },
      {
        title: "Extend the domain",
        text: "Add ticket types, roles, and handlers — file-based routes, the generated route tree, and the OpenAPI spec keep up automatically.",
      },
      {
        title: "Ship it",
        text: "docker compose, Helm, or Nomad. Health probes, metrics, traces, and rate limiting come included.",
      },
    ],
  },
  stack: {
    title: "A deliberately boring, proven stack",
    subtitle:
      "Technologies your team already knows — chosen for the next five years, not the next hype cycle.",
  },
  faq: {
    title: "Frequently asked questions",
    items: [
      {
        q: "What is Go Template?",
        a: "A production-shape starter platform: a Go API and a React admin console with authentication, RBAC, ticketing, background jobs, live streams, and analytics — everything a real internal product needs on day one.",
      },
      {
        q: "Do I need Kubernetes to run it?",
        a: "No. One static Go binary plus Postgres is enough; the frontend is embedded in the binary itself. Docker compose files, a Helm chart, and a Nomad job are included when you outgrow a single host.",
      },
      {
        q: "How is the web frontend served?",
        a: "The React app builds to static assets that are embedded into the Go binary and served same-origin — no CORS, no separate hosting, and the landing page ships as prerendered HTML for crawlers.",
      },
      {
        q: "Which languages is the interface available in?",
        a: "This page and the application UI ship in English, Russian, French, and German — pick one in the header and the choice is remembered.",
      },
      {
        q: "Can I use it as a base for a real product?",
        a: "That is the point. The stack is deliberately boring — Go, Postgres, React, Tailwind — with the plumbing (sessions, RBAC, audit, jobs, observability) already done, so you start on the features that make your product different.",
      },
      {
        q: "Is it free?",
        a: "Yes — clone it, extend it, and ship it. No paid tiers, no license keys, no phone-home.",
      },
    ],
  },
  cta: {
    title: "Ready to look inside?",
    text: "Create an account and walk the whole flow — from filing a ticket to the approval chain — against the seeded demo data.",
    button: "Get started free",
  },
  footer: {
    product: "Product",
    app: "Application",
    apiDocs: "API docs",
    openapiSpec: "OpenAPI spec",
    createAccount: "Create account",
    rights: "© 2026 Go Template",
  },
};

/**
 * Widened structural type (no `as const` above, on purpose): every locale
 * must define every key with plain string values, and the compiler fails
 * the build when a translation misses one.
 */
export type Dictionary = typeof en;
