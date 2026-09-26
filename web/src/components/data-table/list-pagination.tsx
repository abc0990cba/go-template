import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PAGINATION_MODES } from "@/components/data-table/mode-switcher";
import type { PaginationMode } from "@/lib/types";

interface ListPaginationProps {
  mode: PaginationMode;
  page: number;
  hasMore: boolean;
  onFirstPage: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
}

/**
 * ListPagination adapts its controls to the active mode: real page numbers
 * for offset, continuation for cursor and keyset. This difference is the
 * point of the mode switcher — the UI makes the trade-off visible.
 */
export function ListPagination({
  mode,
  page,
  hasMore,
  onFirstPage,
  onPreviousPage,
  onNextPage,
}: ListPaginationProps) {
  const summary = PAGINATION_MODES.find((candidate) => candidate.value === mode)?.label ?? mode;
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p className="text-sm text-muted-foreground">
        {mode === "offset"
          ? `Page ${page}`
          : `Continuing after the ${summary === "Cursor" ? "cursor" : "last row"}`}
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
