import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { toast } from "sonner";
import { Play, RefreshCw } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { useLive } from "@/features/live";
import { useTableView } from "@/shared/hooks/use-paged-list";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api } from "@/lib/api";
import { listSearchSchema, type ListSearch } from "@/shared/lib/search";
import type { Job } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";
import { StatusBadge } from "@/components/status-badge";

export const Route = createFileRoute("/admin/jobs")({
  validateSearch: listSearchSchema,
  staticData: { title: "Jobs" },
  component: JobsPage,
});

function JobsPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { status: liveStatus } = useLive();

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    q: search.q,
    status: search.status,
  };
  const table = useTableView<Job>("jobs", api.listJobs, params, search.view);

  const setSearch = (patch: Partial<ListSearch>) => {
    void navigate({ to: "/admin/jobs", search: { ...search, ...patch } });
  };

  const process = useMutation({
    mutationFn: api.processJob,
    onSuccess: ({ processed }) => {
      toast(
        processed
          ? "Processed one job"
          : "Nothing to process — the distributed queue owns execution while Redis is configured.",
      );
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const columns: ColumnDef<Job>[] = [
    {
      accessorKey: "id",
      header: "ID",
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.id}</span>,
    },
    { accessorKey: "type", header: "Type" },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
    },
    {
      accessorKey: "payload",
      header: "Payload",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="font-mono text-xs text-muted-foreground">{row.original.payload}</span>
      ),
    },
    {
      accessorKey: "error",
      header: "Error",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-xs text-destructive">{row.original.error || "—"}</span>
      ),
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, HH:mm:ss")}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Jobs</h1>
        <p className="text-sm text-muted-foreground">
          The background queue: enrichment batches and report generations moving queued → running →
          done/failed.
        </p>
      </div>

      <ExplainerBanner
        title="Background jobs and live updates"
        howTo={[
          "Enqueue work from the Enrichment or Reports pages; this table shows the lifecycle.",
          "With Redis configured, the worker process consumes jobs via asynq and publishes status changes over Redis pub/sub — the SSE stream relays them here within milliseconds.",
          "Without Redis, “Process one job” runs the oldest queued job inside the API process, so you can watch the full lifecycle in dev.",
        ]}
      >
        Job records live in PostgreSQL; execution happens either in the worker process (Redis/asynq)
        or in-process. The SSE stream carries <code>job.updated</code> events on every transition,
        which invalidate this table's queries — watch rows flip status without a manual refresh
        while the connection badge shows “Live”
        {liveStatus === "live" ? "" : " (currently reconnecting…)"}.
      </ExplainerBanner>

      <DataTable
        columns={columns}
        data={table.rows}
        isPending={table.isPending}
        isPlaceholderData={table.isPlaceholderData}
        emptyState="No jobs match the filters. Enqueue enrichment work or generate a report."
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
          <>
            <Select
              value={search.status ?? "all"}
              onValueChange={(value) =>
                setSearch({
                  status: value === "all" ? undefined : value,
                  page: 1,
                  after: undefined,
                })
              }
            >
              <SelectTrigger className="w-36" aria-label="Filter by status">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="queued">Queued</SelectItem>
                <SelectItem value="running">Running</SelectItem>
                <SelectItem value="done">Done</SelectItem>
                <SelectItem value="failed">Failed</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void queryClient.invalidateQueries({ queryKey: ["jobs"] })}
            >
              <RefreshCw /> Refresh
            </Button>
            <div className="ml-auto">
              <Button size="sm" onClick={() => process.mutate()} disabled={process.isPending}>
                {process.isPending ? <RefreshCw className="size-4 animate-spin" /> : <Play />}
                Process one job
              </Button>
            </div>
          </>
        }
      />
    </div>
  );
}
