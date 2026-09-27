import { keepPreviousData, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { ListParams, Paged } from "@/lib/types";

/** ListParams plus the ticket listing's scope/priority extras. */
export type PagedListParams = Partial<ListParams> & {
  scope?: "my" | "all";
  priority?: string;
  actor?: string;
  action?: string;
  resource?: string;
};

/** Resources whose endpoints speak the shared offset/cursor/keyset vocabulary. */
export type PagedListResource =
  | "users"
  | "jobs"
  | "reports"
  | "enrichment-profiles"
  | "tickets"
  | "approvals"
  | "audit";

interface ListOptions {
  /** Gate the query: only the view currently on screen should hit the network. */
  enabled?: boolean;
}

/**
 * usePagedList wraps any paginated collection endpoint in a TanStack Query.
 * The query key embeds the full parameter set, so switching pagination mode,
 * page, or filters dedupes and caches independently — the dedupe
 * requirement lives here, in one place.
 */
export function usePagedList<T>(
  resource: PagedListResource,
  params: PagedListParams,
  options?: ListOptions,
) {
  return useQuery({
    queryKey: [resource, params],
    queryFn: () => fetchPage<T>(resource, params),
    placeholderData: keepPreviousData,
    enabled: options?.enabled,
  });
}

/**
 * Page parameter of the infinite query: the numeric offset for offset mode,
 * the opaque continuation key for cursor and keyset mode.
 */
export type InfinitePageParam = number | string;

/**
 * useInfiniteList feeds the infinite-scroll view: it walks the same
 * strategy-aware endpoints, appending each fetched page to the feed. The
 * feed accumulates its own position — offset mode steps by the limit,
 * cursor/keyset modes chain next_cursor.
 */
export function useInfiniteList<T>(
  resource: PagedListResource,
  params: PagedListParams,
  options?: ListOptions,
) {
  const mode = params.mode ?? "offset";
  return useInfiniteQuery({
    queryKey: [resource, "infinite", params],
    queryFn: ({ pageParam }) =>
      fetchPage<T>(resource, {
        ...params,
        offset: mode === "offset" ? Number(pageParam) : 0,
        after: mode === "offset" ? "" : String(pageParam),
      }),
    initialPageParam: (mode === "offset" ? 0 : "") as InfinitePageParam,
    // Every strategy answers "is there more" via next_cursor; the page
    // parameter only decides how the next request addresses it.
    getNextPageParam: (lastPage: Paged<T>, _allPages, lastPageParam): InfinitePageParam | undefined => {
      if (!lastPage.next_cursor) return undefined;
      return mode === "offset" ? Number(lastPageParam) + (params.limit ?? 10) : lastPage.next_cursor;
    },
    enabled: options?.enabled,
  });
}

/** Flattens the accumulated pages of an infinite query into table rows. */
export function infiniteRows<T>(result: {
  data?: { pages: Paged<T>[] };
}): T[] {
  return result.data ? result.data.pages.flatMap((page) => page.data) : [];
}

/**
 * useTableView is the one-stop data source for a table page: it runs the
 * paged query or the infinite query depending on the active view (only the
 * visible one hits the network) and reports everything the table's
 * ModeTarget needs. Callers keep page/after state in the URL for the paged
 * view; the infinite feed manages its own position and always starts at
 * the top.
 */
export function useTableView<T>(
  resource: PagedListResource,
  params: PagedListParams,
  view: "paged" | "infinite",
) {
  const isInfinite = view === "infinite";
  const paged = usePagedList<T>(resource, params, { enabled: !isInfinite });
  // The feed ignores the URL's page/after position: offset 0 / no cursor.
  const infinite = useInfiniteList<T>(resource, { ...params, offset: 0, after: "" }, {
    enabled: isInfinite,
  });

  if (isInfinite) {
    return {
      rows: infiniteRows(infinite),
      isPending: infinite.isPending,
      isPlaceholderData: false,
      hasMore: infinite.hasNextPage,
      nextCursor: "",
      loadedCount: infiniteRows(infinite).length,
      isFetchingMore: infinite.isFetchingNextPage,
      loadMore: () => void infinite.fetchNextPage(),
    };
  }
  return {
    rows: paged.data?.data ?? [],
    isPending: paged.isPending,
    isPlaceholderData: paged.isPlaceholderData,
    hasMore: (paged.data?.next_cursor ?? "") !== "",
    nextCursor: paged.data?.next_cursor ?? "",
    loadedCount: paged.data?.data.length ?? 0,
    isFetchingMore: false,
    loadMore: () => {},
  };
}

function fetchPage<T>(resource: PagedListResource, params: PagedListParams): Promise<Paged<T>> {
  switch (resource) {
    case "users":
      return api.listUsers(params) as Promise<Paged<T>>;
    case "jobs":
      return api.listJobs(params) as Promise<Paged<T>>;
    case "reports":
      return api.listReports(params) as Promise<Paged<T>>;
    case "tickets":
      return api.listTickets(params) as Promise<Paged<T>>;
    case "approvals":
      return api.approvals(params) as Promise<Paged<T>>;
    case "audit":
      return api.listAudit(params) as Promise<Paged<T>>;
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
