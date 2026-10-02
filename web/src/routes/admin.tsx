import { Outlet, createFileRoute } from "@tanstack/react-router";
import { AppShell } from "@/app/app-shell";
import { LiveProvider } from "@/features/live";
import { ensurePermission } from "@/shared/lib/auth";

/**
 * /admin is the back-office namespace. The layout guard checks the
 * admin_console:access permission before anything renders — a requester who
 * types the URL lands back in the client app. This is UX layering; every
 * admin API re-checks the permission server-side.
 */
export const Route = createFileRoute("/admin")({
  beforeLoad: ({ location }) => ensurePermission("admin_console", "access", location),
  component: () => (
    <LiveProvider>
      <AppShell>
        <Outlet />
      </AppShell>
    </LiveProvider>
  ),
});
