import { ChevronDown, ChevronLeft, ChevronRight, Loader2 } from "lucide-react";
import { Button } from "@/shared/components/ui/button";
import { PAGINATION_MODES } from "@/shared/components/data-table/mode-switcher";
import type { PaginationMode, ViewMode } from "@/shared/api/pagination";

interface ListPaginationProps {
  mode: PaginationMode;
  view: ViewMode;
  page: number;
  hasMore: boolean;
  onFirstPage: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
  /** Infinite-scroll extras; used when view is "infinite". */
  onLoadMore?: () => void;
  isFetchingMore?: boolean;
  loadedCount?: number;
}

/**
 * ListPagination adapts its controls to the active view and mode: page
 * numbers for the paged view, a load-more button (plus the auto sentinel
 * the DataTable renders) for infinite scroll. The strategy decides whether
 * "Previous" means offset arithmetic or restarting the continuation chain.
 */
export function ListPagination({
  mode,
  view,
  page,
  hasMore,
  onFirstPage,
  onPreviousPage,
  onNextPage,
  onLoadMore,
  isFetchingMore = false,
  loadedCount = 0,
}: ListPaginationProps) {
  const strategyLabel =
    PAGINATION_MODES.find((candidate) => candidate.value === mode)?.label ?? mode;

  if (view === "infinite") {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {loadedCount} {loadedCount === 1 ? "row" : "rows"} loaded
          {hasMore ? ` · more via ${strategyLabel.toLowerCase()}` : " · end of list"}
        </p>
        {hasMore && (
          <Button variant="outline" size="sm" disabled={isFetchingMore} onClick={onLoadMore}>
            {isFetchingMore ? (
              <Loader2 data-slot="icon" className="animate-spin" />
            ) : (
              <ChevronDown data-slot="icon" />
            )}
            Load more
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p className="text-sm text-muted-foreground" aria-live="polite">
        {mode === "offset" ? (
          <>
            Page <span className="font-medium text-foreground">{page}</span>
          </>
        ) : (
          <>
            Page <span className="font-medium text-foreground">{page}</span> ·{" "}
            {strategyLabel === "Cursor" ? "cursor" : "keyset"} continuation
          </>
        )}
      </p>
      <div className="flex items-center gap-2">
        {mode === "offset" ? (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={page <= 1 ? undefined : onPreviousPage}
            >
              <ChevronLeft data-slot="icon" /> Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!hasMore}
              onClick={hasMore ? onNextPage : undefined}
            >
              Next <ChevronRight data-slot="icon" />
            </Button>
          </>
        ) : (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={page <= 1 ? undefined : onFirstPage}
            >
              <ChevronLeft data-slot="icon" /> Start over
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!hasMore}
              onClick={hasMore ? onNextPage : undefined}
            >
              Next page <ChevronRight data-slot="icon" />
            </Button>
          </>
        )}
      </div>
    </div>
  );
}
