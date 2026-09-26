import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";
import { createRouter } from "@tanstack/react-router";
import { routeTree } from "../routeTree.gen";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: (failureCount, error) => {
        // Problem responses are final; only transient network errors retry.
        if (error instanceof ApiError && error.status >= 400) {
          return false;
        }
        return failureCount < 2;
      },
    },
  },
});

export const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
  scrollRestoration: true,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
