import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { toast } from "sonner";
import { FileText, Loader2 } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { useTableView } from "@/hooks/use-paged-list";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { api } from "@/lib/api";
import { listSearchSchema } from "@/lib/search";
import type { Report } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";
import { useState } from "react";

export const Route = createFileRoute("/admin/reports")({
  validateSearch: listSearchSchema,
  staticData: { title: "Reports" },
  component: ReportsPage,
});

function ReportsPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Report | null>(null);

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    q: search.q,
  };
  const table = useTableView<Report>("reports", params, search.view);

  const setSearch = (patch: Partial<typeof search>) => {
    void navigate({ to: "/admin/reports", search: { ...search, ...patch } });
  };

  const generate = useMutation({
    mutationFn: api.generateReport,
    onSuccess: (job) => {
      toast.success(`Report job ${job.id} queued — it finishes in seconds.`);
      void queryClient.invalidateQueries({ queryKey: ["reports"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const columns: ColumnDef<Report>[] = [
    {
      accessorKey: "id",
      header: "ID",
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.id}</span>,
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
    },
    {
      accessorKey: "result",
      header: "Result",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="font-mono text-xs">
          {row.original.result || row.original.error || "—"}
        </span>
      ),
    },
    {
      accessorKey: "created_at",
      header: "Generated",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, yyyy HH:mm")}
        </span>
      ),
    },
    {
      id: "view",
      header: "",
      enableSorting: false,
      cell: ({ row }) => (
        <div className="text-right">
          <Button variant="ghost" size="sm" onClick={() => setSelected(row.original)}>
            <FileText /> View
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Reports</h1>
        <p className="text-sm text-muted-foreground">
          Point-in-time snapshots of the analytics summary.
        </p>
      </div>

      <ExplainerBanner
        title="Snapshot reports vs. live dashboards"
        howTo={[
          "“Generate report” enqueues user.report_generate; the worker snapshots the counters and stores the result line.",
          "A cron (APP_REPORT_CRON, default 3 AM) enqueues the same job nightly when the worker runs with Redis.",
          "Reports are immutable on purpose: a dashboard answers “how is it now”, a report answers “how was it then”.",
        ]}
      >
        Each report stores a one-line summary (users, identities by provider, jobs done) at
        generation time. This page is also the second example of universal pagination: the same mode
        switcher drives GET /api/reports, backed by the
        <code> (created_at DESC, id ASC)</code> index added in migration 000003.
      </ExplainerBanner>

      <DataTable
        columns={columns}
        data={table.rows}
        isPending={table.isPending}
        isPlaceholderData={table.isPlaceholderData}
        emptyState="No reports yet — generate one."
        pagination={{
          mode: search.mode,
          view: search.view,
          page: search.page,
          hasMore: table.hasMore,
          onFirstPage: () => setSearch({ page: 1, after: undefined }),
          onPreviousPage: () => setSearch({ page: Math.max(1, search.page - 1) }),
          onNextPage: () =>
            search.mode === "offset"
              ? setSearch({ page: search.page + 1 })
              : setSearch({ page: search.page + 1, after: table.nextCursor }),
          onModeChange: (mode) => setSearch({ mode, page: 1, after: undefined }),
          onViewChange: (view) => setSearch({ view, page: 1, after: undefined }),
          onLoadMore: table.loadMore,
          isFetchingMore: table.isFetchingMore,
          loadedCount: table.loadedCount,
        }}
        toolbar={
          <div className="ml-auto">
            <Button size="sm" onClick={() => generate.mutate()} disabled={generate.isPending}>
              {generate.isPending ? <Loader2 className="size-4 animate-spin" /> : <FileText />}
              Generate report
            </Button>
          </div>
        }
      />

      <Sheet open={selected !== null} onOpenChange={(open) => !open && setSelected(null)}>
        <SheetContent className="space-y-3">
          <SheetHeader>
            <SheetTitle className="font-mono text-sm">{selected?.id}</SheetTitle>
            <SheetDescription>
              {selected ? format(new Date(selected.created_at), "MMM d, yyyy HH:mm:ss") : ""}
            </SheetDescription>
          </SheetHeader>
          {selected?.error ? (
            <p className="text-sm text-destructive">{selected.error}</p>
          ) : (
            <pre className="rounded-md bg-muted p-3 font-mono text-xs">{selected?.result}</pre>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
