import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

/**
 * A deliberately small i18n layer: flat dictionaries, a provider, and a
 * t() helper. It exists so a template does not need a heavy dependency to
 * feel finished; swap in i18next if keys start proliferating.
 */
export type Locale = "en" | "ru";

const en = {
  "nav.platform": "Platform",
  "nav.administration": "Administration",
  "nav.account": "Account",
  "nav.dashboard": "Dashboard",
  "nav.users": "Users",
  "nav.jobs": "Jobs",
  "nav.enrichment": "Enrichment",
  "nav.reports": "Reports",
  "nav.analytics": "Analytics",
  "nav.roles": "Roles",
  "nav.policies": "Policies",
  "nav.orgs": "Organizations",
  "nav.audit": "Audit Log",
  "nav.settings": "Settings",
  "nav.webhooks": "Webhooks",
  "nav.notifications": "Notifications",
  "nav.profile": "Profile",
  "nav.gettingStarted": "Getting Started",
  "nav.apiDocs": "API Docs",
  "action.logout": "Log out",
  "action.search": "Search…",
  "scope.platform": "Platform (global)",
  "scope.active": "Active scope",
  "banner.impersonating": "Viewing as",
  "banner.returnToAdmin": "Return to admin",
} as const;

const ru: Partial<Record<keyof typeof en, string>> = {
  "nav.platform": "Платформа",
  "nav.administration": "Администрирование",
  "nav.account": "Аккаунт",
  "nav.dashboard": "Дашборд",
  "nav.users": "Пользователи",
  "nav.jobs": "Задачи",
  "nav.enrichment": "Обогащение",
  "nav.reports": "Отчёты",
  "nav.analytics": "Аналитика",
  "nav.roles": "Роли",
  "nav.policies": "Политики",
  "nav.orgs": "Организации",
  "nav.audit": "Журнал аудита",
  "nav.settings": "Настройки",
  "nav.webhooks": "Вебхуки",
  "nav.notifications": "Уведомления",
  "nav.profile": "Профиль",
  "nav.gettingStarted": "Начало работы",
  "nav.apiDocs": "API-документация",
  "action.logout": "Выйти",
  "action.search": "Поиск…",
  "scope.platform": "Платформа (глобально)",
  "scope.active": "Активная область",
  "banner.impersonating": "Вы вошли как",
  "banner.returnToAdmin": "Вернуться к админу",
};

const dictionaries: Record<Locale, Partial<Record<keyof typeof en, string>>> = { en, ru };

export type TKey = keyof typeof en;

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TKey) => string;
}

const I18nContext = createContext<I18nValue>({
  locale: "en",
  setLocale: () => {},
  t: (key) => en[key],
});

const LOCALE_KEY = "app.locale";

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const stored = localStorage.getItem(LOCALE_KEY);
    return stored === "ru" ? "ru" : "en";
  });

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = (next: Locale) => {
    localStorage.setItem(LOCALE_KEY, next);
    setLocaleState(next);
  };

  const t = (key: TKey) => dictionaries[locale][key] ?? en[key];

  return <I18nContext.Provider value={{ locale, setLocale, t }}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  return useContext(I18nContext);
}
