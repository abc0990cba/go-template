import { Outlet, createRootRouteWithContext } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";

/**
 * The root route carries the query client through router context so guards
 * and loaders share one cache. The 404 page is a friendly redirect hint —
 * the sidebar has every real destination.
 */
interface RouterContext {
  queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
  notFoundComponent: NotFound,
});

function NotFound() {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-3 p-6 text-center">
      <p className="text-6xl font-bold tracking-tight">404</p>
      <p className="text-muted-foreground">
        This page does not exist. Use the sidebar to find your way back.
      </p>
      <a href="/dashboard" className="text-sm font-medium underline underline-offset-4">
        Back to the dashboard
      </a>
    </div>
  );
}
