import clsx from "clsx";
import { LOCALES, LOCALE_NAMES, LandingI18nProvider, useLandingI18n } from "./i18n";
import { Cta, Faq, Hero, HowItWorks, StackBand, StatsBand } from "./sections";
import { FeaturesSection } from "./features";
import { useRevealOnScroll } from "./motion";

/**
 * The marketing landing page: fully static copy, semantic markup, and no
 * dependencies besides react + clsx + inlined lucide icons. It is
 * server-rendered to complete HTML at build time (scripts/prerender.mjs)
 * and hydrated in the browser by entry-client.tsx — so the markup must be
 * deterministic: no clock, randomness, or locale-dependent formatting
 * here. The prerendered language is English (the provider mounts as "en"
 * and only a post-hydration effect adopts the visitor's language); the
 * FAQPage JSON-LD inside landing/index.html mirrors the EN FAQ copy.
 */

function LanguageSwitcher() {
  const { locale, setLocale, t } = useLandingI18n();
  return (
    // A fieldset groups the four choices for assistive tech (the lint rule
    // prefers it over role="group"); the default fieldset chrome is reset.
    <fieldset className="m-0 flex items-center rounded-full border border-border bg-card p-0.5">
      {/* The legend names the group for assistive technology. */}
      <legend className="sr-only">{t.lang.label}</legend>
      {LOCALES.map((code) => (
        <button
          key={code}
          type="button"
          onClick={() => setLocale(code)}
          aria-pressed={locale === code}
          aria-label={LOCALE_NAMES[code]}
          title={LOCALE_NAMES[code]}
          className={clsx(
            "rounded-full px-2 py-1 text-xs font-semibold uppercase tracking-wide transition-colors",
            locale === code
              ? "bg-primary text-primary-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {code}
        </button>
      ))}
    </fieldset>
  );
}

function Header() {
  const { t } = useLandingI18n();
  return (
    <header className="sticky top-0 z-40 border-b border-border/60 bg-background/70 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-4 px-4">
        <a href="/" className="flex shrink-0 items-center gap-2" aria-label="Go Template home">
          <span className="flex size-8 items-center justify-center rounded-lg bg-linear-to-br from-sky-500 to-violet-600 text-sm font-bold text-white">
            G
          </span>
          <span className="hidden text-sm font-semibold tracking-tight sm:block">Go Template</span>
        </a>
        <nav aria-label="Site" className="hidden items-center gap-1 md:flex">
          {[
            { href: "#features", label: t.nav.features },
            { href: "#how", label: t.nav.how },
            { href: "#faq", label: t.nav.faq },
          ].map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="rounded-full px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {link.label}
            </a>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2.5">
          <LanguageSwitcher />
          <a
            href="/login"
            className="hidden rounded-full px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground sm:block"
          >
            {t.nav.signIn}
          </a>
          <a
            href="/register"
            className="rounded-full bg-linear-to-r from-sky-600 to-violet-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition-all hover:shadow-md hover:brightness-110"
          >
            {t.nav.getStarted}
          </a>
        </div>
      </div>
    </header>
  );
}

function Footer() {
  const { t } = useLandingI18n();
  return (
    <footer className="border-t border-border/60">
      <div className="mx-auto grid max-w-6xl gap-10 px-4 py-12 sm:grid-cols-[1fr_auto_auto] sm:gap-16">
        <div>
          <div className="flex items-center gap-2">
            <span className="flex size-7 items-center justify-center rounded-lg bg-linear-to-br from-sky-500 to-violet-600 text-xs font-bold text-white">
              G
            </span>
            <span className="text-sm font-semibold">Go Template</span>
          </div>
          <p className="mt-3 max-w-xs text-sm text-muted-foreground">{t.hero.badge}</p>
          <div className="mt-5">
            <LanguageSwitcher />
          </div>
        </div>
        <nav aria-label={t.footer.product} className="text-sm">
          <p className="font-semibold">{t.footer.product}</p>
          <ul className="mt-3 space-y-2 text-muted-foreground">
            <li>
              <a href="#features" className="transition-colors hover:text-foreground">
                {t.nav.features}
              </a>
            </li>
            <li>
              <a href="#how" className="transition-colors hover:text-foreground">
                {t.nav.how}
              </a>
            </li>
            <li>
              <a href="#faq" className="transition-colors hover:text-foreground">
                {t.nav.faq}
              </a>
            </li>
          </ul>
        </nav>
        <nav aria-label={t.footer.app} className="text-sm">
          <p className="font-semibold">{t.footer.app}</p>
          <ul className="mt-3 space-y-2 text-muted-foreground">
            <li>
              <a href="/login" className="transition-colors hover:text-foreground">
                {t.nav.signIn}
              </a>
            </li>
            <li>
              <a href="/register" className="transition-colors hover:text-foreground">
                {t.footer.createAccount}
              </a>
            </li>
            <li>
              <a href="/docs" className="transition-colors hover:text-foreground">
                {t.footer.apiDocs}
              </a>
            </li>
            <li>
              <a href="/openapi.yaml" className="transition-colors hover:text-foreground">
                {t.footer.openapiSpec}
              </a>
            </li>
          </ul>
        </nav>
      </div>
      <div className="border-t border-border/60">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-5">
          <p className="text-xs text-muted-foreground">{t.footer.rights}</p>
          <a
            href="#main"
            className="text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            ↑ Top
          </a>
        </div>
      </div>
    </footer>
  );
}

export function LandingPage() {
  useRevealOnScroll();
  return (
    <LandingI18nProvider>
      <div className="flex min-h-svh flex-col bg-background text-foreground">
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-primary focus:px-3 focus:py-2 focus:text-sm focus:text-primary-foreground"
        >
          Skip to content
        </a>
        <Header />
        <main id="main" className="flex-1">
          <Hero />
          <StatsBand />
          <FeaturesSection />
          <HowItWorks />
          <StackBand />
          <Faq />
          <Cta />
        </main>
        <Footer />
      </div>
    </LandingI18nProvider>
  );
}
