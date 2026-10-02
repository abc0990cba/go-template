import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useMatches, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
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
  Radio,
  Webhook,
  ListTodo,
  LogOut,
  Ticket,
  UserCog,
  Users,
  Workflow,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
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
import { HeaderWidgets, AccountAvatar } from "@/components/header-widgets";
import { api } from "@/lib/api";
import { meQuery } from "@/shared/lib/auth";
import { useCan } from "@/shared/hooks/use-permissions";
import { I18nProvider, useI18n } from "@/shared/lib/i18n";

interface NavItem {
  to: string;
  title: string;
  icon: typeof LayoutDashboard;
  description: string;
  /** Catalog permission gating the item; hidden when the caller lacks it. */
  permission?: [string, string];
  /** i18n key; falls back to the English title. */
  key?: import("@/shared/lib/i18n").TKey;
}

const NAV_MAIN: NavItem[] = [
  {
    to: "/admin",
    key: "nav.dashboard",
    title: "Dashboard",
    icon: LayoutDashboard,
    description: "System counters, charts, live activity",
  },
  {
    to: "/admin/users",
    title: "Users",
    key: "nav.users",
    icon: Users,
    description: "Accounts, search, CRUD",
  },
  {
    to: "/admin/jobs",
    title: "Jobs",
    key: "nav.jobs",
    icon: ListTodo,
    description: "Background queue, live status",
  },
  {
    to: "/admin/enrichment",
    title: "Enrichment",
    icon: Workflow,
    description: "External profile fetching",
  },
  {
    to: "/admin/reports",
    title: "Reports",
    key: "nav.reports",
    icon: FileText,
    description: "Analytics snapshots",
  },
  {
    to: "/admin/ticket-types",
    title: "Ticket Types",
    key: "nav.ticketTypes",
    icon: Ticket,
    description: "Approval chains behind tickets",
    permission: ["ticket_types", "read"],
  },
  {
    to: "/admin/analytics",
    title: "Analytics",
    icon: ChartColumn,
    description: "Event warehouse, live trail",
  },
];

const NAV_ADMIN: NavItem[] = [
  {
    to: "/admin/roles",
    title: "Roles",
    icon: KeySquare,
    description: "Roles and the permission matrix",
    permission: ["roles", "read"],
  },
  {
    to: "/admin/policies",
    title: "Policies",
    icon: FileText,
    description: "Raw policy rules (advanced)",
    permission: ["policies", "read"],
  },
  {
    to: "/admin/orgs",
    title: "Organizations",
    icon: Building2,
    description: "Tenants and their members",
    permission: ["orgs", "read"],
  },
  {
    to: "/admin/audit",
    title: "Audit Log",
    icon: ScrollText,
    description: "Who changed what, when",
    permission: ["audit", "read"],
  },
  {
    to: "/admin/streams",
    title: "Streams",
    key: "nav.streams",
    icon: Radio,
    description: "Outbox and dead letters",
    permission: ["streams", "read"],
  },
  {
    to: "/admin/settings",
    title: "Settings",
    icon: Settings,
    description: "Flags and runtime configuration",
    permission: ["settings", "read"],
  },
  {
    to: "/admin/webhooks",
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
    to: "/admin/getting-started",
    title: "Getting Started",
    icon: BookOpen,
    description: "How this app works",
  },
  {
    to: "/dashboard",
    key: "nav.clientApp",
    title: "Client App",
    icon: Globe,
    description: "The user-facing side of the service",
  },
];

/**
 * AppShell is the authenticated layout: collapsible sidebar, header with
 * theme picker and the SSE status badge, and the ⌘K command palette. It
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
  const { t } = useI18n();

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
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary font-bold text-primary-foreground">
                    G
                  </span>
                  {/* Hidden (not clipped) in icon mode so collapsing never
                      shows half-squeezed text. */}
                  <span className="flex min-w-0 flex-col leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate font-semibold">Go Template</span>
                    <span className="truncate text-xs text-muted-foreground">Admin Console</span>
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
          <header className="flex h-16 items-center gap-2 border-b px-3 md:h-14 md:px-4">
            <SidebarTrigger />
            <Separator orientation="vertical" className="!h-5" />
            <PageTitle />
            <div className="ml-auto flex items-center gap-1">
              <HeaderWidgets onOpenSearch={() => setPaletteOpen(true)} />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <AccountAvatar name={user.name} email={user.email} />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuLabel>
                    <p>{user.name}</p>
                    <p className="text-xs font-normal text-muted-foreground">{user.email}</p>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => goTo("/profile")}>Profile</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => goTo("/")}>Client App</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => goTo("/admin/getting-started")}>
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
          <main className="flex-1 space-y-4 p-3 pb-8 md:p-6">{children}</main>
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
    <span className="min-w-0 truncate text-sm font-medium">
      {(current?.staticData as { title?: string })?.title ?? "Go Template"}
    </span>
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
                  <span className="ml-auto hidden text-xs text-muted-foreground lg:inline">
                    {item.description}
                  </span>
                </CommandItem>
              ))}
              <CommandItem
                onSelect={() => window.open("/docs", "_blank")}
                value="api docs openapi reference"
              >
                <BarChart3 />
                <span>API Docs</span>
                <span className="ml-auto hidden text-xs text-muted-foreground lg:inline">
                  Scalar reference
                </span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

/**
 * ImpersonationBanner sits above everything when an admin is viewing the
 * app as someone else; stopping returns them to their own session.
 */
export function ImpersonationBanner() {
  const { data: me } = useSuspenseQuery(meQuery());
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const stop = useMutation({
    mutationFn: api.stopImpersonation,
    onSuccess: () => void queryClient.invalidateQueries(),
  });
  if (!me.impersonated) return null;
  return (
    <div className="sticky top-0 z-50 flex flex-wrap items-center justify-center gap-x-3 gap-y-1 bg-amber-500 px-4 py-1.5 text-center text-sm font-medium text-black">
      <span className="min-w-0 truncate">
        {t("banner.impersonating")} {me.email} ({me.impersonator_name ?? "admin"})
      </span>
      <Button
        size="sm"
        variant="outline"
        className="h-8 md:h-6"
        onClick={() => stop.mutate()}
        disabled={stop.isPending}
      >
        {t("banner.returnToAdmin")}
      </Button>
    </div>
  );
}
