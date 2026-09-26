import type { ColumnDef } from "@tanstack/react-table";
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
  type SortingState,
} from "@tanstack/react-table";
import { ArrowUpDown } from "lucide-react";
import { useState, type ReactNode } from "react";
import type { PaginationMode } from "@/lib/types";
import { ListPagination } from "@/components/data-table/list-pagination";
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
import { cn } from "@/lib/utils";

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
}

/** ModeTarget is the URL-backed pagination state a page hands to the table. */
export interface ModeTarget {
  mode: PaginationMode;
  page: number;
  hasMore: boolean;
  onFirstPage: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
  onModeChange: (mode: PaginationMode) => void;
}

/**
 * DataTable renders a server-paginated collection with TanStack Table.
 * Pagination and filters live in the URL (the caller passes a ModeTarget
 * backed by route search params); the table itself only handles row
 * rendering plus optional client-side sorting.
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
}: DataTableProps<TData>) {
  const [sortingState, setSortingState] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);

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

  return (
    <div className="space-y-3">
      {toolbar ? <div className="flex flex-wrap items-center gap-2">{toolbar}</div> : null}
      <div
        className={cn(
          "rounded-lg border bg-card transition-opacity",
          isPlaceholderData && "opacity-60",
        )}
      >
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id}>
                    {header.isPlaceholder ? null : header.column.getCanSort() ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="-ml-2 h-7 font-medium"
                        onClick={header.column.getToggleSortingHandler()}
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        <ArrowUpDown className="size-3.5 opacity-50" />
                      </Button>
                    ) : (
                      flexRender(header.column.columnDef.header, header.getContext())
                    )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {isPending ? (
              <SkeletonRows columns={columns.length} />
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-32 text-center text-muted-foreground"
                >
                  {emptyState}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {pagination ? (
        <ListPagination
          mode={pagination.mode}
          page={pagination.page}
          hasMore={pagination.hasMore}
          onFirstPage={pagination.onFirstPage}
          onPreviousPage={pagination.onPreviousPage}
          onNextPage={pagination.onNextPage}
        />
      ) : null}
    </div>
  );
}

function SkeletonRows({ columns }: { columns: number }) {
  return (
    <>
      {Array.from({ length: 5 }, (_unused, index) => (
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
