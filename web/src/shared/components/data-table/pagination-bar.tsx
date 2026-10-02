import { useEffect, useRef } from "react";
import { ListPagination } from "@/shared/components/data-table/list-pagination";
import { ModeSwitcher } from "@/shared/components/data-table/mode-switcher";
import { ViewSwitcher } from "@/shared/components/data-table/view-switcher";
import type { PaginationMode, ViewMode } from "@/shared/api/pagination";

/**
 * ModeTarget is the URL-backed pagination state a page hands to the table:
 * the strategy (offset/cursor/keyset), the presentation view (paged/
 * infinite), and the callbacks that move either forward.
 */
export interface ModeTarget {
  mode: PaginationMode;
  view: ViewMode;
  page: number;
  hasMore: boolean;
  onFirstPage: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
  onModeChange: (mode: PaginationMode) => void;
  onViewChange: (view: ViewMode) => void;
  /** Infinite-scroll controls; used when view is "infinite". */
  onLoadMore?: () => void;
  isFetchingMore?: boolean;
  loadedCount?: number;
}

/**
 * PaginationBar is the shared control surface every paginated table renders:
 * the three-way strategy switcher (offset/cursor/keyset), the view switcher
 * (pages/infinite scroll), and the controls for the active view. DataTable
 * places it under the table; pages with bespoke table markup use it the
 * same way, so the switches look and behave identically everywhere.
 */
export function PaginationBar({ pagination }: { pagination: ModeTarget }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
      <div className="flex flex-wrap items-center gap-3">
        <ModeSwitcher mode={pagination.mode} onModeChange={pagination.onModeChange} />
        <ViewSwitcher view={pagination.view} onViewChange={pagination.onViewChange} />
      </div>
      <ListPagination
        mode={pagination.mode}
        view={pagination.view}
        page={pagination.page}
        hasMore={pagination.hasMore}
        onFirstPage={pagination.onFirstPage}
        onPreviousPage={pagination.onPreviousPage}
        onNextPage={pagination.onNextPage}
        onLoadMore={pagination.onLoadMore}
        isFetchingMore={pagination.isFetchingMore}
        loadedCount={pagination.loadedCount}
      />
    </div>
  );
}

/**
 * InfiniteSentinel watches the space just below the rows; when the user
 * scrolls within 240px of it, the next page loads automatically. A manual
 * "Load more" button stays available in the footer for keyboards and for
 * environments without IntersectionObserver.
 *
 * rootRef names the table's scroll panel when the table scrolls inside a
 * fixed-height container: with the panel as the intersection root, the
 * sentinel fires as the reader approaches the bottom of the PANEL — page
 * scrolling around the table never triggers loads.
 */
export function InfiniteSentinel({
  onLoadMore,
  rootRef,
}: {
  onLoadMore: () => void;
  rootRef?: React.RefObject<HTMLDivElement | null>;
}) {
  const ref = useRef<HTMLDivElement>(null);
  // Keep the observer stable across renders: the callback identity changes
  // every render, but the observation target does not.
  const callbackRef = useRef(onLoadMore);
  useEffect(() => {
    callbackRef.current = onLoadMore;
  });

  useEffect(() => {
    const node = ref.current;
    if (!node || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) callbackRef.current();
      },
      { root: rootRef?.current ?? null, rootMargin: "240px 0px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [rootRef]);

  return <div ref={ref} aria-hidden className="h-px w-full" />;
}
