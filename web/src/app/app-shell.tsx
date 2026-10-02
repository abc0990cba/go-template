import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useMatches, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { BarChart3, ExternalLink, LogOut } from "lucide-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/shared/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/components/ui/dropdown-menu";
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
} from "@/shared/components/ui/sidebar";
import { Separator } from "@/shared/components/ui/separator";
import { HeaderWidgets, AccountAvatar } from "@/app/header-widgets";
import { logout as logoutApi } from "@/features/auth";
import { ImpersonationBanner } from "./impersonation-banner";
import { NAV_MAIN, NAV_ADMIN, NAV_SECONDARY, type NavItem } from "./admin-nav";
import { meQuery } from "@/shared/lib/auth";
import { useCan } from "@/shared/hooks/use-permissions";
import { I18nProvider, useI18n } from "@/shared/lib/i18n";

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
    mutationFn: logoutApi,
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
