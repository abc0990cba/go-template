import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { en, type Dictionary } from "./locales/en";
import { ru } from "./locales/ru";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

/** The UI languages of the landing page. */
export type Locale = "en" | "ru" | "fr" | "de";

/**
 * The landing's i18n layer. The page is prerendered once at build time, so
 * the server render — and therefore everything a crawler sees — is English
 * by construction: the provider always mounts as "en" and only a
 * post-hydration effect adopts the visitor's language (stored choice
 * first, then navigator.languages). That keeps hydration byte-identical
 * and the crawlable HTML stable, while humans get their language instantly.
 */

export const LOCALES: readonly Locale[] = ["en", "ru", "fr", "de"];

export const LOCALE_NAMES: Record<Locale, string> = {
  en: "English",
  ru: "Русский",
  fr: "Français",
  de: "Deutsch",
};

const dictionaries: Record<Locale, Dictionary> = { en, ru, fr, de };

const STORAGE_KEY = "landing.locale";

/**
 * Resolve the visitor's locale: an explicitly stored choice wins, then the
 * first navigator language whose prefix matches a supported locale, then
 * English. Pure so it is unit-testable and hydration-irrelevant (it never
 * runs during SSR).
 */
export function pickLocale(stored: string | null, navigatorLanguages: readonly string[]): Locale {
  if (stored && (LOCALES as readonly string[]).includes(stored)) {
    return stored as Locale;
  }
  for (const language of navigatorLanguages) {
    const prefix = language.toLowerCase().split("-")[0] ?? "";
    if ((LOCALES as readonly string[]).includes(prefix)) {
      return prefix as Locale;
    }
  }
  return "en";
}

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  /** Short alias for the whole dictionary: `t.hero.badge`, `t.faq.items`. */
  t: Dictionary;
}

const I18nContext = createContext<I18nValue>({
  locale: "en",
  setLocale: () => {},
  t: en,
});

export function LandingI18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");

  // Adopt the visitor's language only after hydration: the first client
  // render must match the server markup byte for byte. The synchronous
  // setState is the point — one deliberate re-render right after hydration,
  // before the visitor has seen or interacted with anything.
  useEffect(() => {
    const preferred = pickLocale(
      localStorage.getItem(STORAGE_KEY),
      navigator.languages ?? [navigator.language],
    );
    // oxlint-disable-next-line set-state-in-effect -- deliberate one-time post-hydration adoption
    setLocaleState(preferred);
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = dictionaries[locale].meta.title;
  }, [locale]);

  const setLocale = (next: Locale) => {
    localStorage.setItem(STORAGE_KEY, next);
    setLocaleState(next);
  };

  return (
    <I18nContext.Provider value={{ locale, setLocale, t: dictionaries[locale] }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useLandingI18n(): I18nValue {
  return useContext(I18nContext);
}
