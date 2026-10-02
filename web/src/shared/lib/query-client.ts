import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "@/shared/api/core";

/**
 * The one query client of the SPA: guards, loaders, and pages share this
 * cache. Living in shared/ keeps the route guards (shared) and the router
 * instance (app) from importing each other.
 */
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
