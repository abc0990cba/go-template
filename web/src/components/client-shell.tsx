import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import {
  Bell,
  FileCheck2,
  Home,
  KeySquare,
  LogOut,
  ShieldCheck,
  Ticket,
  UserCog,
} from "lucide-react";
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
import { AccountAvatar, HeaderWidgets } from "@/components/header-widgets";
import { DemoAccountPicker } from "@/components/demo-account-picker";
import { useCan, useMe } from "@/hooks/use-permissions";
import { api } from "@/lib/api";
import { I18nProvider, useI18n } from "@/lib/i18n";

// Header chrome shared with the admin shell.
import { ImpersonationBanner } from "@/components/app-shell";

const NAV_LINKS = [
  { to: "/", key: "nav.home", icon: Home, permission: undefined },
  { to: "/tickets", key: "nav.tickets", icon: Ticket, permission: ["tickets", "read"] },
  { to: "/approvals", key: "nav.approvals", icon: FileCheck2, permission: ["tickets", "approve"] },
  { to: "/notifications", key: "nav.notifications", icon: Bell, permission: undefined },
  { to: "/profile", key: "nav.profile", icon: UserCog, permission: undefined },
] as const;

/**
 * ClientShell is the user-facing layout: a light top navbar (Home, tickets,
 * approvals) instead of the admin console's sidebar. The separation is
 * deliberate — a requester never sees the admin chrome, and the /admin
 * guard never lets them in even if they type the URL. The right-hand widget
 * cluster is the same HeaderWidgets the admin header renders, so the two
 * top bars stay visually identical.
 */
export function ClientShell({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  const me = useMe();
  const can = useCan();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [switchOpen, setSwitchOpen] = useState(false);

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.removeQueries();
      void navigate({ to: "/login" });
    },
  });

  const links = NAV_LINKS.filter(
    (link) => !link.permission || can(link.permission[0], link.permission[1]),
  );

  return (
    <I18nProvider>
      <ImpersonationBanner />
      <div className="flex min-h-svh flex-col bg-muted/30">
        <header className="sticky top-0 z-40 border-b bg-background">
          <div className="mx-auto flex h-14 max-w-7xl items-center gap-2 px-3 md:px-4">
            <Link to="/" className="flex shrink-0 items-center gap-2">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary font-bold text-primary-foreground">
                G
              </span>
              <span className="hidden flex-col leading-tight sm:flex">
                <span className="text-sm font-semibold">Go Template</span>
                <span className="text-[10px] text-muted-foreground">{t("shell.clientArea")}</span>
              </span>
            </Link>

            <nav className="ml-2 hidden min-w-0 items-center gap-1 md:flex">
              {links.map((link) => (
                <Link
                  key={link.to}
                  to={link.to}
                  className="flex items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  activeProps={{ className: "bg-muted text-foreground font-medium" }}
                >
                  <link.icon className="size-4" />
                  {t(link.key)}
                </Link>
              ))}
            </nav>

            <div className="ml-auto flex min-w-0 items-center gap-1">
              <HeaderWidgets />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <AccountAvatar name={me.name} email={me.email} />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuLabel>
                    <p>{me.name}</p>
                    <p className="text-xs font-normal text-muted-foreground">{me.email}</p>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => setSwitchOpen(true)}>
                    <KeySquare /> {t("demo.switch")}
                  </DropdownMenuItem>
                  {can("admin_console", "access") && (
                    <DropdownMenuItem onClick={() => void navigate({ to: "/admin" })}>
                      <ShieldCheck /> {t("shell.adminConsole")}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem onClick={() => void navigate({ to: "/profile" })}>
                    <UserCog /> {t("nav.profile")}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onClick={() => logout.mutate()}>
                    <LogOut /> {t("action.logout")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          {/* Mobile navigation: the same links as a scrollable row. */}
          <nav className="flex items-center gap-1 overflow-x-auto border-t px-3 py-1.5 md:hidden">
            {links.map((link) => (
              <Link
                key={link.to}
                to={link.to}
                className="flex items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                activeProps={{ className: "bg-muted text-foreground font-medium" }}
              >
                <link.icon className="size-3.5" />
                {t(link.key)}
              </Link>
            ))}
          </nav>
        </header>

        <main className="mx-auto w-full max-w-7xl flex-1 space-y-4 p-4 md:p-6">{children}</main>

        <footer className="border-t bg-background py-3">
          <div className="mx-auto flex max-w-7xl items-center justify-between px-4 text-xs text-muted-foreground">
            <span>Go Template — {t("shell.clientArea")}</span>
            {can("admin_console", "access") && (
              <Link to="/admin" className="underline underline-offset-4 hover:text-foreground">
                {t("shell.adminConsole")}
              </Link>
            )}
          </div>
        </footer>
      </div>

      <Dialog open={switchOpen} onOpenChange={setSwitchOpen}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("demo.switch")}</DialogTitle>
            <DialogDescription>{t("demo.switchHint")}</DialogDescription>
          </DialogHeader>
          <DemoAccountPicker />
        </DialogContent>
      </Dialog>
    </I18nProvider>
  );
}
