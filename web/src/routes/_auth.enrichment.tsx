import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { toast } from "sonner";
import { CloudDownload, Loader2 } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { usePagedList } from "@/hooks/use-paged-list";
import { useLive } from "@/hooks/use-event-stream";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { listSearchSchema } from "@/lib/search";
import type { EnrichmentProfile } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";
import { useState } from "react";

export const Route = createFileRoute("/_auth/enrichment")({
  validateSearch: listSearchSchema,
  staticData: { title: "Enrichment" },
  component: EnrichmentPage,
});

function EnrichmentPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { jobs } = useLive();
  const [count, setCount] = useState("10");

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
  };
  const profiles = usePagedList<EnrichmentProfile>("enrichment-profiles", params);

  const setSearch = (patch: Partial<typeof search>) => {
    void navigate({ to: "/enrichment", search: { ...search, ...patch } });
  };

  const enqueue = useMutation({
    mutationFn: () =>
      api.enqueueEnrichment(Math.min(Math.max(Number.parseInt(count, 10) || 10, 1), 100)),
    onSuccess: (job) => {
      toast.success(`Queued ${job.id} — watch it move to running below or on the Jobs page.`);
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["enrichment-profiles"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const columns: ColumnDef<EnrichmentProfile>[] = [
    {
      accessorKey: "user_id",
      header: "User",
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.user_id || "—"}</span>,
    },
    { accessorKey: "source", header: "Source" },
    {
      accessorKey: "payload",
      header: "Profile",
      enableSorting: false,
      cell: ({ row }) => (
        <pre className="max-w-80 truncate font-mono text-xs text-muted-foreground">
          {row.original.payload}
        </pre>
      ),
    },
    {
      accessorKey: "created_at",
      header: "Fetched at",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, HH:mm:ss")}
        </span>
      ),
    },
  ];

  const runningJobs = jobs
    .filter((job) => job.status === "running" || job.status === "queued")
    .slice(0, 5);

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Enrichment</h1>
        <p className="text-sm text-muted-foreground">
          Fetch synthetic external profiles and browse what was stored.
        </p>
      </div>

      <ExplainerBanner
        title="What enrichment does"
        howTo={[
          "Pick a count between 1 and 100 and press “Fetch profiles” — one user.enrich_batch job is queued.",
          "The worker calls randomuser.me with bounded concurrency (APP_MAX_CONCURRENCY) and 3 retries per profile.",
          "Every fetched profile is appended to user_enrichment_profiles; the table below pages through them in the selected mode.",
        ]}
      >
        This is the app's concrete “talk to an external API reliably” example: bounded fan-out,
        retries with backoff, permanent errors short-circuiting, and results persisted with the raw
        payload. Failures land in the event trail as <code>job_failed</code> and show up on the Jobs
        page with the error text.
      </ExplainerBanner>

      <div className="flex flex-wrap items-center gap-3 rounded-lg border bg-card p-4">
        <label htmlFor="enrich-count" className="flex items-center gap-2 text-sm font-medium">
          <CloudDownload className="size-4 text-muted-foreground" />
          Profiles to fetch
        </label>
        <Input
          id="enrich-count"
          type="number"
          min={1}
          max={100}
          value={count}
          onChange={(event) => setCount(event.target.value)}
          className="w-24"
        />
        <Button onClick={() => enqueue.mutate()} disabled={enqueue.isPending}>
          {enqueue.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
          Fetch profiles
        </Button>
        {runningJobs.length > 0 ? (
          <p className="text-sm text-muted-foreground">
            In flight right now: {runningJobs.length} job(s) queued or running.
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">
            Nothing in flight; enqueue to see live progress on Jobs.
          </p>
        )}
      </div>

      <DataTable
        columns={columns}
        data={profiles.data?.data}
        isPending={profiles.isPending}
        isPlaceholderData={profiles.isPlaceholderData}
        emptyState="No profiles stored yet — fetch a batch to populate this trail."
        pagination={{
          mode: search.mode,
          page: search.page,
          hasMore: (profiles.data?.next_cursor ?? "") !== "",
          onFirstPage: () => setSearch({ page: 1, after: undefined }),
          onPreviousPage: () => setSearch({ page: Math.max(1, search.page - 1) }),
          onNextPage: () =>
            search.mode === "offset"
              ? setSearch({ page: search.page + 1 })
              : setSearch({ page: search.page + 1, after: profiles.data?.next_cursor }),
          onModeChange: (mode) => setSearch({ mode, page: 1, after: undefined }),
        }}
      />
    </div>
  );
}
