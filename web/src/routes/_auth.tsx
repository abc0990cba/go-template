import { Outlet, createFileRoute } from "@tanstack/react-router";
import { AppShell } from "@/components/app-shell";
import { LiveProvider } from "@/hooks/use-event-stream";
import { ensureAuth } from "@/lib/auth";

/**
 * _auth is the pathless authenticated layout: it resolves the session via
 * the query client (deduped across navigations) and mounts the app shell
 * plus the single SSE connection.
 */
export const Route = createFileRoute("/_auth")({
  beforeLoad: ({ location }) => ensureAuth(location),
  component: () => (
    <LiveProvider>
      <AppShell>
        <Outlet />
      </AppShell>
    </LiveProvider>
  ),
});
