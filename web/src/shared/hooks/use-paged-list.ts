import { keepPreviousData, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { ListParams, Paged } from "@/shared/api/pagination";

/** ListParams plus the ticket listing's scope/priority extras. */
export type PagedListParams = Partial<ListParams> & {
  scope?: "my" | "all";
  priority?: string;
  actor?: string;
  action?: string;
  resource?: string;
  resource_id?: string;
};

interface ListOptions {
  /** Gate the query: only the view currently on screen should hit the network. */
  enabled?: boolean;
}

/**
 * usePagedList wraps any paginated collection endpoint in a TanStack Query.
 * The caller names the list (query key) and hands in its feature's fetch
 * function — this hook stays transport-agnostic, shared knows no features.
 * The key embeds the full parameter set, so switching pagination mode,
 * page, or filters dedupes and caches independently.
 */
export function usePagedList<T>(
  key: string,
  fetchPage: (params: PagedListParams) => Promise<Paged<T>>,
  params: PagedListParams,
  options?: ListOptions,
) {
  return useQuery({
    queryKey: [key, params],
    queryFn: () => fetchPage(params),
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
  key: string,
  fetchPage: (params: PagedListParams) => Promise<Paged<T>>,
  params: PagedListParams,
  options?: ListOptions,
) {
  const mode = params.mode ?? "offset";
  return useInfiniteQuery({
    queryKey: [key, "infinite", params],
    queryFn: ({ pageParam }) =>
      fetchPage({
        ...params,
        offset: mode === "offset" ? Number(pageParam) : 0,
        after: mode === "offset" ? "" : String(pageParam),
      }),
    initialPageParam: (mode === "offset" ? 0 : "") as InfinitePageParam,
    // Every strategy answers "is there more" via next_cursor; the page
    // parameter only decides how the next request addresses it.
    getNextPageParam: (
      lastPage: Paged<T>,
      _allPages,
      lastPageParam,
    ): InfinitePageParam | undefined => {
      if (!lastPage.next_cursor) return undefined;
      return mode === "offset"
        ? Number(lastPageParam) + (params.limit ?? 10)
        : lastPage.next_cursor;
    },
    enabled: options?.enabled,
  });
}

/** Flattens the accumulated pages of an infinite query into table rows. */
export function infiniteRows<T>(result: { data?: { pages: Paged<T>[] } }): T[] {
  // A nil slice on the wire arrives as null; treat it as an empty page.
  return result.data ? result.data.pages.flatMap((page) => page.data ?? []) : [];
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
  key: string,
  fetchPage: (params: PagedListParams) => Promise<Paged<T>>,
  params: PagedListParams,
  view: "paged" | "infinite",
) {
  const isInfinite = view === "infinite";
  const paged = usePagedList<T>(key, fetchPage, params, { enabled: !isInfinite });
  // The feed ignores the URL's page/after position: offset 0 / no cursor.
  const infinite = useInfiniteList<T>(
    key,
    fetchPage,
    { ...params, offset: 0, after: "" },
    {
      enabled: isInfinite,
    },
  );

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
    loadedCount: paged.data?.data?.length ?? 0,
    isFetchingMore: false,
    loadMore: () => {},
  };
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
