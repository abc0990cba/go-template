import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { ListParams, Paged } from "@/lib/types";

/**
 * usePagedList wraps any paginated collection endpoint in a TanStack Query.
 * The query key embeds the full parameter set, so switching pagination mode,
 * page, or filters dedupes and caches independently — the dedupe
 * requirement lives here, in one place.
 */
export function usePagedList<T>(
  resource: "users" | "jobs" | "reports" | "enrichment-profiles",
  params: Partial<ListParams>,
) {
  return useQuery({
    queryKey: [resource, params],
    queryFn: () => fetchPage<T>(resource, params),
    placeholderData: keepPreviousData,
  });
}

function fetchPage<T>(resource: string, params: Partial<ListParams>): Promise<Paged<T>> {
  switch (resource) {
    case "users":
      return api.listUsers(params) as Promise<Paged<T>>;
    case "jobs":
      return api.listJobs(params) as Promise<Paged<T>>;
    case "reports":
      return api.listReports(params) as Promise<Paged<T>>;
    default:
      return api.listEnrichmentProfiles(params) as Promise<Paged<T>>;
  }
}

/** useDebounced delays a rapidly changing value (e.g. search input). */
export function useDebounced<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}
