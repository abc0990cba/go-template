import type { ColumnDef } from "@tanstack/react-table";
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
  type SortingState,
} from "@tanstack/react-table";
import { ArrowUpDown, Loader2 } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";
import {
  InfiniteSentinel,
  PaginationBar,
  type ModeTarget,
} from "@/components/data-table/pagination-bar";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/shared/lib/utils";

/**
 * Per-column responsive classes: pages set `meta.cellClass` / `meta.headerClass`
 * to hide or restyle columns at breakpoints (e.g. hide a Created column on
 * phones) without DataTable knowing the schema.
 */
interface ColumnMeta {
  cellClass?: string;
  headerClass?: string;
}

interface DataTableProps<TData> {
  columns: ColumnDef<TData>[];
  data: TData[] | undefined;
  isPending: boolean;
  isPlaceholderData: boolean;
  emptyState: ReactNode;
  /** Sorting keeps the demo honest: column sorts stay client-side. */
  sorting?: boolean;
  toolbar?: ReactNode;
  pagination?: ModeTarget;
  /**
   * Makes whole rows clickable (detail-page navigation). Clicks on buttons
   * and links inside a row stay theirs: the guard skips them.
   */
  onRowClick?: (row: TData) => void;
}

/**
 * ModeTarget (re-exported from pagination-bar) is the URL-backed pagination
 * state a page hands to the table: the strategy (offset/cursor/keyset), the
 * presentation view (paged/infinite), and the callbacks that move either
 * forward.
 */
export type { ModeTarget } from "@/components/data-table/pagination-bar";

/**
 * DataTable renders a server-paginated collection with TanStack Table.
 * Pagination and filters live in the URL (the caller passes a ModeTarget
 * backed by route search params); the table itself only handles row
 * rendering plus optional client-side sorting. The PaginationBar under the
 * table exposes the two switchers — pagination strategy and view — so every
 * table in the app supports offset, cursor, and keyset paging, plus
 * infinite scroll, with a single control surface.
 *
 * In infinite view the table scrolls INSIDE a fixed-height panel: the feed
 * grows within the panel, the sentinel observes that panel, and the page
 * below the table keeps its place no matter how many rows load. The panel
 * height also caps paged view, so very long pages pan inside the table
 * instead of pushing the rest of the page away.
 */
export function DataTable<TData>({
  columns,
  data,
  isPending,
  isPlaceholderData,
  emptyState,
  sorting = true,
  toolbar,
  pagination,
  onRowClick,
}: DataTableProps<TData>) {
  const [sortingState, setSortingState] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const scrollRef = useRef<HTMLDivElement>(null);

  const table = useReactTable({
    data: data ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
    onSortingChange: setSortingState,
    onColumnFiltersChange: setColumnFilters,
    state: { sorting: sortingState, columnFilters },
    manualPagination: true,
    enableSorting: sorting,
  });

  const onLoadMore = pagination?.onLoadMore;
  const autoLoad =
    pagination?.view === "infinite" &&
    pagination.hasMore &&
    !pagination.isFetchingMore &&
    onLoadMore;

  return (
    <div className="space-y-3">
      {toolbar ? <div className="flex flex-wrap items-center gap-2">{toolbar}</div> : null}
      <div
        ref={scrollRef}
        className={cn(
          "rounded-lg border bg-card transition-opacity",
          "max-h-[min(72svh,42rem)] overflow-y-auto overscroll-contain",
          pagination?.view === "infinite" && "border-ring/30",
          isPlaceholderData && "opacity-60",
        )}
      >
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-card shadow-[0_1px_0_0_var(--border)]">
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => {
                  const meta = header.column.columnDef.meta as ColumnMeta | undefined;
                  return (
                    <TableHead key={header.id} className={meta?.headerClass}>
                      {header.isPlaceholder ? null : header.column.getCanSort() ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="-ml-2 h-8 font-medium md:h-7"
                          onClick={header.column.getToggleSortingHandler()}
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <ArrowUpDown className="size-3.5 opacity-50" />
                        </Button>
                      ) : (
                        flexRender(header.column.columnDef.header, header.getContext())
                      )}
                    </TableHead>
                  );
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {isPending ? (
              <SkeletonRows columns={columns.length} />
            ) : table.getRowModel().rows.length === 0 ? null : (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  className={onRowClick ? "cursor-pointer" : undefined}
                  onClick={
                    onRowClick
                      ? (event) => {
                          if (
                            (event.target as HTMLElement).closest("button, a, [role='menuitem']")
                          ) {
                            return; // buttons and links keep their own action
                          }
                          onRowClick(row.original);
                        }
                      : undefined
                  }
                >
                  {row.getVisibleCells().map((cell) => {
                    const meta = cell.column.columnDef.meta as ColumnMeta | undefined;
                    return (
                      <TableCell key={cell.id} className={meta?.cellClass}>
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    );
                  })}
                </TableRow>
              ))
            )}
            {pagination?.view === "infinite" && pagination.isFetchingMore ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="py-3 text-center">
                  <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" /> Loading more…
                  </span>
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
        {/* The empty state lives outside the table: a pannable table's width
            follows its columns, and a centered message would stretch and clip
            past the panel on phones. Panel-width keeps the prose readable. */}
        {!isPending && table.getRowModel().rows.length === 0 ? (
          <div className="flex h-32 items-center justify-center px-4 text-center text-sm text-muted-foreground">
            {emptyState}
          </div>
        ) : null}
        {autoLoad ? <InfiniteSentinel rootRef={scrollRef} onLoadMore={onLoadMore} /> : null}
      </div>
      {pagination ? <PaginationBar pagination={pagination} /> : null}
    </div>
  );
}

function SkeletonRows({ columns, rows = 5 }: { columns: number; rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }, (_unused, index) => (
        <TableRow key={index}>
          {Array.from({ length: columns }, (_, cell) => (
            <TableCell key={cell}>
              <Skeleton className="h-4 w-full max-w-40" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  );
}
