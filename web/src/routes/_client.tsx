import { Outlet, createFileRoute } from "@tanstack/react-router";
import { ClientShell } from "@/components/client-shell";
import { LiveProvider } from "@/hooks/use-event-stream";
import { ensureAuth } from "@/lib/auth";

/**
 * _client is the pathless layout of the user-facing application: everything
 * an ordinary account needs (home, tickets, approvals, notifications,
 * profile) mounts inside the light ClientShell under the root paths.
 */
export const Route = createFileRoute("/_client")({
  beforeLoad: ({ location }) => ensureAuth(location),
  component: () => (
    <LiveProvider>
      <ClientShell>
        <Outlet />
      </ClientShell>
    </LiveProvider>
  ),
});
