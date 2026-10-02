import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import * as React from "react";
import { useState } from "react";
import { Building2, Globe, Search } from "lucide-react";
import { Button } from "@/shared/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/shared/components/ui/dropdown-menu";
import { IdentityChip } from "@/shared/components/identity-chip";
import { LiveBadge } from "@/features/live";
import { NotificationBell } from "@/features/notifications";
import { ThemeMenu } from "@/shared/components/theme-menu";
import { UserAvatar } from "@/shared/components/user-avatar";
import { setActiveOrg } from "@/shared/api/core";
import { meQuery } from "@/shared/lib/auth";
import { useI18n, type Locale } from "@/shared/lib/i18n";

/**
 * HeaderWidgets is the shared right-hand cluster of both shells (admin
 * console and client area). Rendering one component in both places is what
 * keeps the top bars consistent: same order, same sizes, same responsive
 * behavior — compact icon buttons on phones, more detail as space allows.
 * The caller appends the account-avatar menu right after it.
 */
export function HeaderWidgets({ onOpenSearch }: { onOpenSearch?: () => void }) {
  const { locale, setLocale } = useI18n();
  return (
    <>
      <IdentityChip />
      {onOpenSearch ? <SearchButton onClick={onOpenSearch} /> : null}
      <OrgSwitcher />
      <NotificationBell />
      {/* The stream status is chrome, not content: phones get the space back. */}
      <LiveBadge className="hidden md:inline-flex" />
      <LanguageToggle locale={locale} setLocale={setLocale} />
      <ThemeMenu />
    </>
  );
}

function SearchButton({ onClick }: { onClick: () => void }) {
  const { t } = useI18n();
  return (
    <Button
      variant="outline"
      size="sm"
      className="gap-2 text-muted-foreground"
      aria-label={t("action.search")}
      onClick={onClick}
    >
      <Search className="size-3.5" />
      <span className="hidden lg:inline">{t("action.search")}</span>
      <kbd className="pointer-events-none hidden rounded border bg-muted px-1.5 font-mono text-[10px] lg:inline-block">
        ⌘K
      </kbd>
    </Button>
  );
}

/**
 * OrgSwitcher selects the active Casbin domain: a tenant or the platform.
 * The choice rides every API call as X-Org-Id and survives reloads.
 */
export function OrgSwitcher() {
  const { data: me } = useSuspenseQuery(meQuery());
  const orgT = useI18n().t;
  const queryClient = useQueryClient();
  const [active, setActive] = useState(() => localStorage.getItem("app.activeOrg") ?? "");
  const current = me.orgs.find((org) => org.id === active);
  const scopeLabel = current ? current.name : "Platform scope";

  const pick = (orgId: string) => {
    setActiveOrg(orgId);
    setActive(orgId);
    // Everything the caller sees is domain-scoped; refetch it.
    void queryClient.invalidateQueries();
  };

  if (me.orgs.length === 0 && !me.permissions.includes("orgs:read")) {
    return null;
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5 px-2 text-muted-foreground"
          title={`Active scope: ${scopeLabel}`}
        >
          {current ? <Building2 className="size-3.5" /> : <Globe className="size-3.5" />}
          <span className="hidden max-w-32 truncate sm:inline">{scopeLabel}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>Active scope</DropdownMenuLabel>
        <DropdownMenuItem onClick={() => pick("")}>
          <Globe /> {orgT("scope.platform")}
        </DropdownMenuItem>
        {me.orgs.map((org) => (
          <DropdownMenuItem key={org.id} onClick={() => pick(org.id)}>
            <Building2 /> {org.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** LanguageToggle flips the UI between English and Russian. */
export function LanguageToggle({
  locale,
  setLocale,
}: {
  locale: Locale;
  setLocale: (l: Locale) => void;
}) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="text-xs font-semibold text-muted-foreground"
      aria-label="Toggle language"
      onClick={() => setLocale(locale === "en" ? "ru" : "en")}
    >
      {locale === "en" ? "EN" : "RU"}
    </Button>
  );
}

/** Shared initials helper for the shells' avatar fallbacks. */
export function initials(nameOrEmail: string) {
  const parts = nameOrEmail.split(/[\s@.]+/).filter(Boolean);
  const first = parts.at(0)?.charAt(0) ?? "?";
  const second = parts.at(1)?.charAt(0) ?? "";
  return (first + second).toUpperCase();
}

/**
 * AccountAvatar is the round profile button. Both shells render it inside
 * `DropdownMenuTrigger asChild`, so every trigger prop (onClick, aria-*,
 * ref) must flow through to the underlying Button — dropping the rest
 * spread here silently disconnects the account menu.
 */
export function AccountAvatar({
  name,
  email,
  ...props
}: { name: string; email: string } & React.ComponentProps<typeof Button>) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="rounded-full p-0"
      aria-label="Account menu"
      {...props}
    >
      <UserAvatar name={name} email={email} className="size-8" fallbackClassName="text-xs" />
    </Button>
  );
}
