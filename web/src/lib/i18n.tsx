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
  "nav.streams": "Streams",
  "nav.audit": "Audit Log",
  "nav.settings": "Settings",
  "nav.webhooks": "Webhooks",
  "nav.notifications": "Notifications",
  "nav.profile": "Profile",
  "nav.gettingStarted": "Getting Started",
  "nav.apiDocs": "API Docs",
  "nav.tickets": "My Tickets",
  "nav.approvals": "Approvals",
  "nav.home": "Home",
  "nav.clientApp": "Client App",
  "nav.ticketTypes": "Ticket Types",
  "shell.clientArea": "Client workspace",
  "shell.adminConsole": "Admin console",
  "chip.viewingAs": "Viewing as",
  "demo.account": "Account",
  "demo.roles": "Roles",
  "demo.password": "Password",
  "demo.whatToCheck": "What to check",
  "demo.area": "Area",
  "demo.areaClient": "Client",
  "demo.areaAdmin": "Admin",
  "demo.signin": "Sign in",
  "demo.switch": "Switch demo account",
  "demo.switchHint": "Pick a seeded account and sign in as that role with one click.",
  "demo.signedInAs": "Signed in as",
  "ticket.status.draft": "Draft",
  "ticket.status.in_review": "In review",
  "ticket.status.approved": "Approved",
  "ticket.status.completed": "Completed",
  "ticket.status.rejected": "Rejected by approver",
  "ticket.status.withdrawn": "Withdrawn by requester",
  "ticket.action.submit": "Submit",
  "ticket.action.approve": "Approve",
  "ticket.action.reject": "Reject",
  "ticket.action.withdraw": "Withdraw",
  "ticket.action.complete": "Deliver result",
  "ticket.action.reopen": "Rework",
  "ticket.banner.withdrawn": "Withdrawn by the requester",
  "ticket.banner.rejected": "Rejected by the approver",
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
  "nav.streams": "Потоки",
  "nav.settings": "Настройки",
  "nav.webhooks": "Вебхуки",
  "nav.notifications": "Уведомления",
  "nav.profile": "Профиль",
  "nav.gettingStarted": "Начало работы",
  "nav.apiDocs": "API-документация",
  "nav.tickets": "Мои заявки",
  "nav.approvals": "Согласование",
  "nav.home": "Главная",
  "nav.clientApp": "Клиентское приложение",
  "nav.ticketTypes": "Типы заявок",
  "shell.clientArea": "Клиентский кабинет",
  "shell.adminConsole": "Админ-панель",
  "chip.viewingAs": "Вы вошли как",
  "demo.account": "Аккаунт",
  "demo.roles": "Роли",
  "demo.password": "Пароль",
  "demo.whatToCheck": "Что проверить",
  "demo.area": "Область",
  "demo.areaClient": "Клиент",
  "demo.areaAdmin": "Админ",
  "demo.signin": "Войти",
  "demo.switch": "Сменить демо-аккаунт",
  "demo.switchHint": "Выберите засеянный аккаунт и войдите под нужной ролью одним кликом.",
  "demo.signedInAs": "Вы вошли как",
  "ticket.status.draft": "Черновик",
  "ticket.status.in_review": "На согласовании",
  "ticket.status.approved": "Согласована",
  "ticket.status.completed": "Завершена",
  "ticket.status.rejected": "Отклонена согласующим",
  "ticket.status.withdrawn": "Отозвана заявителем",
  "ticket.action.submit": "Подать",
  "ticket.action.approve": "Согласовать",
  "ticket.action.reject": "Отклонить",
  "ticket.action.withdraw": "Отозвать",
  "ticket.action.complete": "Сдать результат",
  "ticket.action.reopen": "Доработать",
  "ticket.banner.withdrawn": "Отозвана заявителем",
  "ticket.banner.rejected": "Отклонена согласующим",
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
