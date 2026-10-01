import {
  ArrowRightIcon,
  BarChart3Icon,
  CheckIcon,
  ChevronDownIcon,
  RocketIcon,
  ScrollTextIcon,
  ShieldCheckIcon,
  UsersIcon,
  WorkflowIcon,
} from "./icons";

/**
 * The marketing landing page: fully static copy, semantic markup, and no
 * dependencies besides react + lucide icons. It is server-rendered to
 * complete HTML at build time (scripts/prerender.mjs) and hydrated in the
 * browser by entry-client.tsx — so the markup must be deterministic: no
 * clock, randomness, or locale-dependent formatting here. Copy changes are
 * mirrored in the FAQPage JSON-LD inside landing/index.html.
 */

const FEATURES = [
  {
    icon: ShieldCheckIcon,
    title: "Authentication & MFA",
    text: "Cookie sessions, OAuth links, TOTP two-factor, email verification, invitations, and API keys — wired end to end.",
  },
  {
    icon: UsersIcon,
    title: "RBAC with Casbin",
    text: "Per-organization roles, platform grants, and an audit trail behind every state change.",
  },
  {
    icon: ScrollTextIcon,
    title: "Tickets & approvals",
    text: "Multi-step approval chains with notifications, withdrawal, rework, and result delivery.",
  },
  {
    icon: WorkflowIcon,
    title: "Jobs & live streams",
    text: "Queued background jobs, Kafka/NATS enrichment, SSE push updates, and a dead-letter console.",
  },
  {
    icon: BarChart3Icon,
    title: "Reports & analytics",
    text: "ClickHouse-backed dashboards, scheduled reports, and CSV exports produced by the job queue.",
  },
  {
    icon: RocketIcon,
    title: "One binary deploys",
    text: "The React console embeds into the Go binary — one container, one port, zero CORS setup.",
  },
] as const;

const STEPS = [
  {
    title: "Clone and configure",
    text: "Environment-driven config, docker compose for Postgres and friends, and a seeded demo dataset to explore.",
  },
  {
    title: "Extend the domain",
    text: "Add ticket types, roles, and handlers — file-based routes, the generated route tree, and the OpenAPI spec keep up.",
  },
  {
    title: "Ship it",
    text: "docker compose up, Helm, or Nomad. Health probes, metrics, traces, and rate limiting come included.",
  },
] as const;

const FAQS = [
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
    q: "Can I use it as a base for a real product?",
    a: "That is the point. The stack is deliberately boring — Go, Postgres, React, Tailwind — with the plumbing (sessions, RBAC, audit, jobs, observability) already done, so you start on the features that make your product different.",
  },
  {
    q: "Is it free?",
    a: "Yes — clone it, extend it, and ship it. No paid tiers, no license keys, no phone-home.",
  },
] as const;

export function LandingPage() {
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-primary focus:px-3 focus:py-2 focus:text-sm focus:text-primary-foreground"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-40 border-b border-border/60 bg-background/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-4">
          <a href="/" className="flex shrink-0 items-center gap-2" aria-label="Go Template home">
            <span className="flex size-8 items-center justify-center rounded-lg bg-linear-to-br from-sky-500 to-violet-600 text-sm font-bold text-white">
              G
            </span>
            <span className="text-sm font-semibold tracking-tight">Go Template</span>
          </a>
          <nav aria-label="Site" className="hidden items-center gap-1 md:flex">
            <a
              href="#features"
              className="rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              Features
            </a>
            <a
              href="#how"
              className="rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              How it works
            </a>
            <a
              href="#faq"
              className="rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              FAQ
            </a>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <a
              href="/login"
              className="rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Sign in
            </a>
            <a
              href="/register"
              className="rounded-md bg-linear-to-r from-sky-600 to-violet-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:from-sky-500 hover:to-violet-500"
            >
              Get started
            </a>
          </div>
        </div>
      </header>

      <main id="main" className="flex-1">
        {/* Hero: the h1 is the LCP element — plain text, zero image requests,
            which is the fastest Core Web Vitals profile a page can have. */}
        <section className="relative overflow-hidden">
          <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
            <div className="absolute inset-0 bg-[radial-gradient(48rem_32rem_at_115%_-10%,rgba(139,92,246,0.16),transparent)]" />
            <div className="absolute inset-0 bg-[radial-gradient(42rem_30rem_at_-15%_110%,rgba(14,165,233,0.14),transparent)]" />
          </div>
          <div className="mx-auto grid max-w-6xl items-center gap-10 px-4 py-16 md:py-24 lg:grid-cols-[minmax(0,1fr)_minmax(0,520px)]">
            <div>
              <p className="mb-4 inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs font-medium text-muted-foreground">
                <span className="size-1.5 rounded-full bg-emerald-500" />
                Open-source starter platform
              </p>
              <h1 className="text-4xl font-bold tracking-tight text-balance sm:text-5xl">
                The Go + React platform that ships as{" "}
                <span className="bg-linear-to-r from-sky-600 to-violet-600 bg-clip-text text-transparent">
                  one binary
                </span>
              </h1>
              <p className="mt-5 max-w-xl text-lg text-muted-foreground">
                Authentication, RBAC, ticketing with approvals, background jobs, live streams, and
                analytics — a production-shape foundation you can clone, extend, and deploy in
                minutes.
              </p>
              <div className="mt-8 flex flex-wrap items-center gap-3">
                <a
                  href="/register"
                  className="inline-flex items-center gap-2 rounded-md bg-linear-to-r from-sky-600 to-violet-600 px-5 py-2.5 text-sm font-medium text-white shadow-md transition-colors hover:from-sky-500 hover:to-violet-500"
                >
                  Create a free account <ArrowRightIcon className="size-4" />
                </a>
                <a
                  href="/login"
                  className="inline-flex items-center rounded-md border border-border bg-card px-5 py-2.5 text-sm font-medium transition-colors hover:bg-muted"
                >
                  Explore the demo
                </a>
              </div>
              <ul className="mt-8 flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted-foreground">
                {[
                  "One container, one port",
                  "No CORS, no separate hosting",
                  "Health probes included",
                ].map((item) => (
                  <li key={item} className="flex items-center gap-1.5">
                    <CheckIcon className="size-4 text-emerald-500" /> {item}
                  </li>
                ))}
              </ul>
            </div>

            {/* Terminal card: static text styled as a console — looks like a
                product shot without shipping a single image byte. */}
            <div className="overflow-hidden rounded-xl border border-border bg-card shadow-2xl shadow-primary/10">
              <div className="flex items-center gap-1.5 border-b border-border px-4 py-3">
                <span className="size-3 rounded-full bg-red-400" />
                <span className="size-3 rounded-full bg-amber-400" />
                <span className="size-3 rounded-full bg-emerald-400" />
                <span className="ml-2 font-mono text-xs text-muted-foreground">terminal</span>
              </div>
              <pre className="overflow-x-auto p-5 font-mono text-[13px] leading-6">
                <code>
                  <span className="text-muted-foreground">$</span> git clone
                  github.com/your-org/go-template{"\n"}
                  <span className="text-muted-foreground">$</span> docker compose up -d{"\n"}
                  <span className="text-emerald-600 dark:text-emerald-400">✓ postgres</span>{" "}
                  <span className="text-muted-foreground">healthy</span>
                  {"\n"}
                  <span className="text-emerald-600 dark:text-emerald-400">✓ api</span>{" "}
                  <span className="text-muted-foreground">http://localhost:8080</span>
                  {"\n"}
                  <span className="text-muted-foreground">$</span> open http://localhost:8080{"\n"}
                  <span className="text-violet-600 dark:text-violet-400">
                    → admin console, embedded.
                  </span>
                </code>
              </pre>
            </div>
          </div>
        </section>

        <section
          id="features"
          aria-labelledby="features-title"
          className="border-t border-border/60"
        >
          <div className="mx-auto max-w-6xl px-4 py-16 md:py-24">
            <h2 id="features-title" className="text-3xl font-bold tracking-tight">
              Everything an internal product needs, on day one
            </h2>
            <p className="mt-3 max-w-2xl text-muted-foreground">
              Not a hello-world: the plumbing is done, so the first commit you write is a feature.
            </p>
            <ul className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {FEATURES.map((feature) => (
                <li key={feature.title} className="rounded-xl border border-border bg-card p-5">
                  <span className="mb-4 flex size-10 items-center justify-center rounded-lg bg-linear-to-br from-sky-500/10 to-violet-600/10">
                    <feature.icon className="size-5 text-primary" aria-hidden />
                  </span>
                  <h3 className="font-semibold">{feature.title}</h3>
                  <p className="mt-1.5 text-sm text-muted-foreground">{feature.text}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section
          id="how"
          aria-labelledby="how-title"
          className="border-t border-border/60 bg-muted/40"
        >
          <div className="mx-auto max-w-6xl px-4 py-16 md:py-24">
            <h2 id="how-title" className="text-3xl font-bold tracking-tight">
              From clone to production in three steps
            </h2>
            <ol className="mt-10 grid gap-6 md:grid-cols-3">
              {STEPS.map((step, index) => (
                <li
                  key={step.title}
                  className="relative rounded-xl border border-border bg-card p-5"
                >
                  <span
                    aria-hidden
                    className="mb-4 flex size-8 items-center justify-center rounded-full bg-linear-to-br from-sky-600 to-violet-600 text-sm font-bold text-white"
                  >
                    {index + 1}
                  </span>
                  <h3 className="font-semibold">{step.title}</h3>
                  <p className="mt-1.5 text-sm text-muted-foreground">{step.text}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section id="faq" aria-labelledby="faq-title" className="border-t border-border/60">
          <div className="mx-auto max-w-3xl px-4 py-16 md:py-24">
            <h2 id="faq-title" className="text-3xl font-bold tracking-tight">
              Frequently asked questions
            </h2>
            {/* Native <details>: interactive without a line of JavaScript,
                and every answer is real DOM text for crawlers. */}
            <div className="mt-8 space-y-3">
              {FAQS.map((faq) => (
                <details key={faq.q} className="group rounded-xl border border-border bg-card">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-4 font-medium [&::-webkit-details-marker]:hidden">
                    {faq.q}
                    <ChevronDownIcon
                      className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180"
                      aria-hidden
                    />
                  </summary>
                  <p className="px-4 pb-4 text-sm text-muted-foreground">{faq.a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>

        <section className="border-t border-border/60 bg-muted/40" aria-labelledby="cta-title">
          <div className="mx-auto max-w-6xl px-4 py-16 text-center md:py-20">
            <h2 id="cta-title" className="text-3xl font-bold tracking-tight">
              Ready to look inside?
            </h2>
            <p className="mx-auto mt-3 max-w-xl text-muted-foreground">
              Create an account and walk the whole flow — from filing a ticket to the approval chain
              — against the seeded demo data.
            </p>
            <a
              href="/register"
              className="mt-8 inline-flex items-center gap-2 rounded-md bg-linear-to-r from-sky-600 to-violet-600 px-6 py-3 text-sm font-medium text-white shadow-md transition-colors hover:from-sky-500 hover:to-violet-500"
            >
              Get started free <ArrowRightIcon className="size-4" />
            </a>
          </div>
        </section>
      </main>

      <footer className="border-t border-border/60">
        <div className="mx-auto flex max-w-6xl flex-col items-start justify-between gap-6 px-4 py-10 sm:flex-row sm:items-center">
          <div className="flex items-center gap-2">
            <span className="flex size-7 items-center justify-center rounded-lg bg-linear-to-br from-sky-500 to-violet-600 text-xs font-bold text-white">
              G
            </span>
            <span className="text-sm font-semibold">Go Template</span>
          </div>
          <nav
            aria-label="Footer"
            className="flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted-foreground"
          >
            <a href="/docs" className="transition-colors hover:text-foreground">
              API docs
            </a>
            <a href="/openapi.yaml" className="transition-colors hover:text-foreground">
              OpenAPI spec
            </a>
            <a href="/login" className="transition-colors hover:text-foreground">
              Sign in
            </a>
            <a href="/register" className="transition-colors hover:text-foreground">
              Create account
            </a>
          </nav>
          <p className="text-xs text-muted-foreground">© 2026 Go Template</p>
        </div>
      </footer>
    </div>
  );
}
