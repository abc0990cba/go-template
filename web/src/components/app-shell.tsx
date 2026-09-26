import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useMatches, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import {
  BarChart3,
  Bell,
  BookOpen,
  Building2,
  ChartColumn,
  ExternalLink,
  FileText,
  Globe,
  KeySquare,
  LayoutDashboard,
  ScrollText,
  Settings,
  Webhook,
  ListTodo,
  LogOut,
  Moon,
  Search,
  Sun,
  UserCog,
  Users,
  Workflow,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { api, setActiveOrg } from "@/lib/api";
import { meQuery } from "@/lib/auth";
import { useCan } from "@/hooks/use-permissions";
import { LiveBadge } from "@/components/live-badge";
import { NotificationBell } from "@/components/notification-bell";
import { I18nProvider, useI18n, type Locale } from "@/lib/i18n";

interface NavItem {
  to: string;
  title: string;
  icon: typeof LayoutDashboard;
  description: string;
  /** Catalog permission gating the item; hidden when the caller lacks it. */
  permission?: [string, string];
  /** i18n key; falls back to the English title. */
  key?: import("@/lib/i18n").TKey;
}

const NAV_MAIN: NavItem[] = [
  {
    to: "/",
    key: "nav.dashboard",
    title: "Dashboard",
    icon: LayoutDashboard,
    description: "Counters, charts, live activity",
  },
  {
    to: "/users",
    title: "Users",
    key: "nav.users",
    icon: Users,
    description: "Accounts, search, CRUD",
  },
  {
    to: "/jobs",
    title: "Jobs",
    key: "nav.jobs",
    icon: ListTodo,
    description: "Background queue, live status",
  },
  {
    to: "/enrichment",
    title: "Enrichment",
    icon: Workflow,
    description: "External profile fetching",
  },
  {
    to: "/reports",
    title: "Reports",
    key: "nav.reports",
    icon: FileText,
    description: "Analytics snapshots",
  },
  {
    to: "/analytics",
    title: "Analytics",
    icon: ChartColumn,
    description: "Event warehouse, live trail",
  },
];

const NAV_ADMIN: NavItem[] = [
  {
    to: "/roles",
    title: "Roles",
    icon: KeySquare,
    description: "Roles and the permission matrix",
    permission: ["roles", "read"],
  },
  {
    to: "/policies",
    title: "Policies",
    icon: FileText,
    description: "Raw policy rules (advanced)",
    permission: ["policies", "read"],
  },
  {
    to: "/orgs",
    title: "Organizations",
    icon: Building2,
    description: "Tenants and their members",
    permission: ["orgs", "read"],
  },
  {
    to: "/audit",
    title: "Audit Log",
    icon: ScrollText,
    description: "Who changed what, when",
    permission: ["audit", "read"],
  },
  {
    to: "/settings",
    title: "Settings",
    icon: Settings,
    description: "Flags and runtime configuration",
    permission: ["settings", "read"],
  },
  {
    to: "/webhooks",
    title: "Webhooks",
    icon: Webhook,
    description: "Outgoing event subscriptions",
    permission: ["webhooks", "read"],
  },
];

const NAV_SECONDARY: NavItem[] = [
  {
    to: "/notifications",
    title: "Notifications",
    key: "nav.notifications",
    icon: Bell,
    description: "Your in-app alerts",
  },
  {
    to: "/profile",
    title: "Profile",
    key: "nav.profile",
    icon: UserCog,
    description: "Identities, sessions, logout",
  },
  {
    to: "/getting-started",
    title: "Getting Started",
    icon: BookOpen,
    description: "How this app works",
  },
];

/**
 * OrgSwitcher selects the active Casbin domain: a tenant or the platform.
 * The choice rides every API call as X-Org-Id and survives reloads.
 */
function OrgSwitcher() {
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
        <Button variant="outline" size="sm" className="gap-2 text-muted-foreground">
          {current ? <Building2 className="size-3.5" /> : <Globe className="size-3.5" />}
          <span className="hidden max-w-40 truncate sm:inline">{scopeLabel}</span>
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

/**
 * AppShell is the authenticated layout: collapsible sidebar, header with
 * theme toggle and the SSE status badge, and the ⌘K command palette. It
 * renders inside the _auth route so every authenticated page shares it.
 */
export function AppShell({ children }: { children: React.ReactNode }) {
  const { data: user } = useSuspenseQuery(meQuery());
  const can = useCan();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [paletteOpen, setPaletteOpen] = useState(false);

  const visible = (items: NavItem[]) =>
    items.filter((item) => !item.permission || can(...item.permission));

  const navMain = visible(NAV_MAIN);
  const navAdmin = visible(NAV_ADMIN);
  const navSecondary = visible(NAV_SECONDARY);
  const navAll = [...navMain, ...navAdmin, ...navSecondary];
  const { locale, setLocale, t } = useI18n();

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "k" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setPaletteOpen((previous) => !previous);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.removeQueries();
      void navigate({ to: "/login" });
    },
  });

  const goTo = (to: string) => {
    setPaletteOpen(false);
    void navigate({ to });
  };

  return (
    <I18nProvider>
      <SidebarProvider>
        <ImpersonationBanner />
        <Sidebar collapsible="icon">
          <SidebarHeader>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton size="lg" onClick={() => goTo("/")} tooltip="Go Template Admin">
                  <span className="flex size-8 items-center justify-center rounded-lg bg-primary font-bold text-primary-foreground">
                    G
                  </span>
                  <span className="flex flex-col leading-tight">
                    <span className="font-semibold">Go Template</span>
                    <span className="text-xs text-muted-foreground">Admin Console</span>
                  </span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarHeader>
          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupLabel>{t("nav.platform")}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {navMain.map((item) => (
                    <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton
                        tooltip={`${item.title} — ${item.description}`}
                        onClick={() => goTo(item.to)}
                      >
                        <item.icon />
                        <span>{item.key ? t(item.key) : item.title}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
            {navAdmin.length > 0 && (
              <SidebarGroup>
                <SidebarGroupLabel>{t("nav.administration")}</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {navAdmin.map((item) => (
                      <SidebarMenuItem key={item.to}>
                        <SidebarMenuButton
                          tooltip={`${item.title} — ${item.description}`}
                          onClick={() => goTo(item.to)}
                        >
                          <item.icon />
                          <span>{item.key ? t(item.key) : item.title}</span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            )}
            <SidebarGroup>
              <SidebarGroupLabel>{t("nav.account")}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {navSecondary.map((item) => (
                    <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton
                        tooltip={`${item.title} — ${item.description}`}
                        onClick={() => goTo(item.to)}
                      >
                        <item.icon />
                        <span>{item.key ? t(item.key) : item.title}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      tooltip="OpenAPI reference served by the backend"
                      onClick={() => window.open("/docs", "_blank")}
                    >
                      <BarChart3 />
                      <span>API Docs</span>
                      <ExternalLink className="ml-auto size-3 opacity-60" />
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>
          <SidebarFooter>
            <div className="px-2 pb-2 text-xs text-muted-foreground group-data-[collapsible=icon]:hidden">
              <p>Signed in as</p>
              <p className="truncate font-medium text-foreground">{user.email}</p>
            </div>
          </SidebarFooter>
        </Sidebar>

        <SidebarInset>
          <header className="flex h-14 items-center gap-3 border-b px-4">
            <SidebarTrigger />
            <Separator orientation="vertical" className="!h-5" />
            <PageTitle />
            <div className="ml-auto flex items-center gap-2">
              <OrgSwitcher />
              <NotificationBell />
              <LiveBadge />
              <Button
                variant="outline"
                size="sm"
                className="gap-2 text-muted-foreground"
                onClick={() => setPaletteOpen(true)}
              >
                <Search className="size-3.5" />
                <span className="hidden sm:inline">{t("action.search")}</span>
                <kbd className="pointer-events-none rounded border bg-muted px-1.5 font-mono text-[10px]">
                  ⌘K
                </kbd>
              </Button>
              <LanguageToggle locale={locale} setLocale={setLocale} />
              <ThemeToggle />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="rounded-full"
                    aria-label="Account menu"
                  >
                    <Avatar className="size-8">
                      <AvatarFallback>{initials(user.name || user.email)}</AvatarFallback>
                    </Avatar>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>
                    <p>{user.name}</p>
                    <p className="text-xs font-normal text-muted-foreground">{user.email}</p>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => goTo("/profile")}>Profile</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => goTo("/getting-started")}>
                    Getting Started
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onClick={() => logout.mutate()}>
                    <LogOut /> {t("action.logout")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </header>
          <main className="flex-1 space-y-4 p-4 md:p-6">{children}</main>
        </SidebarInset>

        <CommandPalette
          open={paletteOpen}
          onOpenChange={setPaletteOpen}
          onNavigate={goTo}
          items={navAll}
        />
      </SidebarProvider>
    </I18nProvider>
  );
}

/**
 * PageTitle derives the header title from the active route's static data;
 * routes declare `staticData: { title: '…' }`.
 */
function PageTitle() {
  const matches = useMatches();
  const current = matches
    .toReversed()
    .find((match) => (match.staticData as { title?: string }).title !== undefined);
  return (
    <span className="text-sm font-medium">
      {(current?.staticData as { title?: string })?.title ?? "Go Template"}
    </span>
  );
}

/** LanguageToggle flips the UI between English and Russian. */
function LanguageToggle({ locale, setLocale }: { locale: Locale; setLocale: (l: Locale) => void }) {
  return (
    <Button
      variant="ghost"
      size="sm"
      className="text-xs text-muted-foreground"
      aria-label="Toggle language"
      onClick={() => setLocale(locale === "en" ? "ru" : "en")}
    >
      {locale === "en" ? "EN" : "RU"}
    </Button>
  );
}

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const isDark = resolvedTheme === "dark";
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label="Toggle dark mode"
      onClick={() => setTheme(isDark ? "light" : "dark")}
    >
      {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  );
}

function CommandPalette({
  open,
  onOpenChange,
  onNavigate,
  items,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNavigate: (to: string) => void;
  items: NavItem[];
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0" showCloseButton={false}>
        <DialogHeader className="sr-only">
          <DialogTitle>Command palette</DialogTitle>
          <DialogDescription>Jump to any page</DialogDescription>
        </DialogHeader>
        <Command>
          <CommandInput placeholder="Jump to…" />
          <CommandList>
            <CommandEmpty>Nothing found.</CommandEmpty>
            <CommandGroup heading="Navigation">
              {items.map((item) => (
                <CommandItem
                  key={item.to}
                  onSelect={() => onNavigate(item.to)}
                  value={`${item.title} ${item.description}`}
                >
                  <item.icon />
                  <span>{item.title}</span>
                  <span className="ml-auto text-xs text-muted-foreground">{item.description}</span>
                </CommandItem>
              ))}
              <CommandItem
                onSelect={() => window.open("/docs", "_blank")}
                value="api docs openapi reference"
              >
                <BarChart3 />
                <span>API Docs</span>
                <span className="ml-auto text-xs text-muted-foreground">Scalar reference</span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

function initials(nameOrEmail: string) {
  const parts = nameOrEmail.split(/[\s@.]+/).filter(Boolean);
  const first = parts.at(0)?.charAt(0) ?? "?";
  const second = parts.at(1)?.charAt(0) ?? "";
  return (first + second).toUpperCase();
}

/**
 * ImpersonationBanner sits above everything when an admin is viewing the
 * app as someone else; stopping returns them to their own session.
 */
function ImpersonationBanner() {
  const { data: me } = useSuspenseQuery(meQuery());
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const stop = useMutation({
    mutationFn: api.stopImpersonation,
    onSuccess: () => void queryClient.invalidateQueries(),
  });
  if (!me.impersonated) return null;
  return (
    <div className="sticky top-0 z-50 flex items-center justify-center gap-3 bg-amber-500 px-4 py-1.5 text-sm font-medium text-black">
      {t("banner.impersonating")} {me.email} ({me.impersonator_name ?? "admin"})
      <Button
        size="sm"
        variant="outline"
        className="h-6"
        onClick={() => stop.mutate()}
        disabled={stop.isPending}
      >
        {t("banner.returnToAdmin")}
      </Button>
    </div>
  );
}
