import { useLandingI18n } from "./i18n";
import { ArrowRightIcon, CheckIcon, ChevronDownIcon } from "./icons";

/** The stack chips are brand names — identical in every locale. */
const STACK = [
  "Go",
  "React",
  "TypeScript",
  "PostgreSQL",
  "Redis",
  "Kafka",
  "NATS JetStream",
  "ClickHouse",
  "Docker",
  "Kubernetes",
  "Tailwind CSS",
  "OpenTelemetry",
] as const;

/** Hero: the h1 is the LCP element — plain text, zero image requests. The
    entrance is pure CSS (hero-in classes), so it plays without JS too. */
export function Hero() {
  const { t } = useLandingI18n();
  return (
    <section className="relative overflow-hidden">
      <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
        <div className="hero-grid absolute inset-0" />
        <div className="orb absolute -top-32 right-[-12%] size-[36rem] rounded-full bg-[radial-gradient(circle,rgba(139,92,246,0.16),transparent_65%)] blur-2xl" />
        <div className="orb orb-alt absolute left-[-14%] top-[35%] size-[32rem] rounded-full bg-[radial-gradient(circle,rgba(14,165,233,0.14),transparent_65%)] blur-2xl" />
      </div>
      <div className="mx-auto grid max-w-6xl items-center gap-12 px-4 pb-16 pt-14 md:pb-24 md:pt-20 lg:grid-cols-[minmax(0,1fr)_minmax(0,520px)]">
        <div>
          <p className="hero-in hero-in-1 mb-5 inline-flex items-center gap-2.5 rounded-full border border-border bg-card/70 px-3 py-1 text-xs font-medium text-muted-foreground backdrop-blur">
            <span className="relative flex size-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60 motion-reduce:animate-none" />
              <span className="relative inline-flex size-2 rounded-full bg-emerald-500" />
            </span>
            {t.hero.badge}
          </p>
          <h1 className="hero-in hero-in-2 text-4xl font-bold tracking-tight text-balance sm:text-5xl">
            {t.hero.titlePre}{" "}
            <span className="bg-linear-to-r from-sky-500 to-violet-600 bg-clip-text text-transparent">
              {t.hero.titleAccent}
            </span>
            {t.hero.titlePost ? ` ${t.hero.titlePost}` : ""}
          </h1>
          <p className="hero-in hero-in-3 mt-5 max-w-xl text-lg leading-8 text-muted-foreground">
            {t.hero.subtitle}
          </p>
          <div className="hero-in hero-in-4 mt-8 flex flex-wrap items-center gap-3">
            <a
              href="/register"
              className="group inline-flex items-center gap-2 rounded-full bg-linear-to-r from-sky-600 to-violet-600 px-5 py-2.5 text-sm font-medium text-white shadow-lg shadow-primary/25 transition-all hover:shadow-xl hover:shadow-primary/30 hover:brightness-110"
            >
              {t.hero.ctaPrimary}
              <ArrowRightIcon className="size-4 transition-transform duration-200 group-hover:translate-x-0.5" />
            </a>
            <a
              href="/login"
              className="inline-flex items-center rounded-full border border-border bg-card px-5 py-2.5 text-sm font-medium transition-colors hover:bg-muted"
            >
              {t.hero.ctaSecondary}
            </a>
          </div>
          <ul className="hero-in hero-in-5 mt-8 flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted-foreground">
            {t.hero.checks.map((item) => (
              <li key={item} className="flex items-center gap-1.5">
                <CheckIcon className="size-4 shrink-0 text-emerald-500" aria-hidden /> {item}
              </li>
            ))}
          </ul>
        </div>

        {/* Terminal card: static text styled as a console — looks like a
            product shot without shipping a single image byte. */}
        <div className="hero-in hero-in-5 overflow-hidden rounded-2xl border border-border bg-card shadow-2xl shadow-primary/10">
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
              <span className="text-muted-foreground">$</span> open http://localhost:8080
              {"\n"}
              <span className="text-violet-600 dark:text-violet-400">
                → admin console, embedded.
              </span>
              <span
                aria-hidden
                className="terminal-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-violet-500"
              />
            </code>
          </pre>
        </div>
      </div>
    </section>
  );
}

export function StatsBand() {
  const { t } = useLandingI18n();
  return (
    <section className="border-y border-border/60 bg-muted/40">
      <div className="mx-auto grid max-w-6xl grid-cols-2 gap-8 px-4 py-10 text-center md:grid-cols-4">
        {t.stats.map((stat) => (
          <div key={stat.label}>
            <p className="bg-linear-to-r from-sky-500 to-violet-600 bg-clip-text text-3xl font-bold tracking-tight text-transparent">
              {stat.value}
            </p>
            <p className="mt-1 text-sm text-muted-foreground">{stat.label}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

export function HowItWorks() {
  const { t } = useLandingI18n();
  return (
    <section id="how" aria-labelledby="how-title" className="border-t border-border/60 bg-muted/40">
      <div className="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div data-reveal className="max-w-2xl">
          <h2 id="how-title" className="text-3xl font-bold tracking-tight text-balance">
            {t.how.title}
          </h2>
          <p className="mt-3 text-muted-foreground">{t.how.subtitle}</p>
        </div>
        <ol className="mt-12 grid gap-4 md:grid-cols-3">
          {t.how.steps.map((step, index) => (
            <li
              key={step.title}
              data-reveal
              className="rounded-2xl border border-border bg-card p-6"
            >
              <span
                aria-hidden
                className="mb-4 flex size-9 items-center justify-center rounded-full bg-linear-to-br from-sky-600 to-violet-600 text-sm font-bold text-white"
              >
                {index + 1}
              </span>
              <h3 className="font-semibold tracking-tight">{step.title}</h3>
              <p className="mt-1.5 text-sm leading-6 text-muted-foreground">{step.text}</p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}

export function StackBand() {
  const { t } = useLandingI18n();
  return (
    <section aria-labelledby="stack-title" className="border-t border-border/60">
      <div data-reveal className="mx-auto max-w-6xl px-4 py-16 text-center md:py-20">
        <h2 id="stack-title" className="text-2xl font-bold tracking-tight sm:text-3xl">
          {t.stack.title}
        </h2>
        <p className="mx-auto mt-3 max-w-xl text-muted-foreground">{t.stack.subtitle}</p>
        <ul className="mt-8 flex flex-wrap justify-center gap-2.5">
          {STACK.map((name) => (
            <li
              key={name}
              className="rounded-full border border-border bg-card px-3.5 py-1.5 text-sm text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
            >
              {name}
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

export function Faq() {
  const { t } = useLandingI18n();
  return (
    <section id="faq" aria-labelledby="faq-title" className="border-t border-border/60">
      <div className="mx-auto max-w-3xl px-4 py-16 md:py-24">
        <h2 id="faq-title" data-reveal className="text-3xl font-bold tracking-tight">
          {t.faq.title}
        </h2>
        {/* Native <details>: interactive without a line of JavaScript, and
            every answer is real DOM text for crawlers. */}
        <div className="mt-8 space-y-3">
          {t.faq.items.map((faq) => (
            <details
              key={faq.q}
              data-reveal
              className="faq-item group rounded-2xl border border-border bg-card transition-colors hover:border-primary/40"
            >
              <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-4 font-medium [&::-webkit-details-marker]:hidden">
                {faq.q}
                <ChevronDownIcon
                  className="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-180"
                  aria-hidden
                />
              </summary>
              <p className="px-4 pb-4 text-sm leading-6 text-muted-foreground">{faq.a}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}

export function Cta() {
  const { t } = useLandingI18n();
  return (
    <section className="border-t border-border/60" aria-labelledby="cta-title">
      <div className="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div
          data-reveal
          className="relative overflow-hidden rounded-3xl bg-linear-to-br from-sky-600 via-violet-600 to-violet-700 px-6 py-14 text-center shadow-2xl shadow-primary/30 sm:px-12"
        >
          <div
            aria-hidden
            className="pointer-events-none absolute -top-24 left-1/2 size-96 -translate-x-1/2 rounded-full bg-white/10 blur-3xl"
          />
          <h2 id="cta-title" className="text-3xl font-bold tracking-tight text-balance sm:text-4xl">
            {t.cta.title}
          </h2>
          <p className="mx-auto mt-3 max-w-xl text-white/85">{t.cta.text}</p>
          <a
            href="/register"
            className="group mt-8 inline-flex items-center gap-2 rounded-full bg-white px-6 py-3 text-sm font-semibold text-violet-700 shadow-lg transition-all hover:bg-white/90 hover:shadow-xl"
          >
            {t.cta.button}
            <ArrowRightIcon className="size-4 transition-transform duration-200 group-hover:translate-x-0.5" />
          </a>
        </div>
      </div>
    </section>
  );
}
