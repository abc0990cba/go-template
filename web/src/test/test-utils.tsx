import { QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { routeTree } from "@/routeTree.gen";
import { queryClient } from "@/shared/lib/query-client";
import type { Me } from "@/shared/types/core";
import { Toaster } from "@/shared/components/ui/sonner";
import { meFixture } from "./mocks/handlers";

/**
 * renderRoute boots the real route tree (guards, loaders, search schemas)
 * against the shared query client, at `path` of an in-memory history —
 * the integration harness for page-level tests.
 *
 * The QueryClientProvider and sonner Toaster ride along because the app
 * mounts both once globally (main.tsx); without them, queries crash and
 * toast calls render nothing to assert on.
 */
export function renderRoute(path: string) {
  const history = createMemoryHistory({ initialEntries: [path] });
  const router = createRouter({ routeTree, context: { queryClient }, history });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster />
    </QueryClientProvider>,
  );
  return { router, history };
}

/**
 * seedMe signs the shared session cache in as the given account. Guards,
 * shells, and useCan all read the ["me"] query; seeding it keeps the
 * authenticated render path off the network for the session probe.
 */
export function seedMe(overrides: Partial<Me> = {}): Me {
  const me = meFixture(overrides);
  queryClient.setQueryData(["me"], me);
  return me;
}
