import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { ChevronDown, ChevronRight, Search } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import {
  InfiniteSentinel,
  PaginationBar,
} from "@/components/data-table/pagination-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDebounced, useTableView } from "@/hooks/use-paged-list";
import { ensurePermission } from "@/lib/auth";
import { listSearchSchema, type ListSearch } from "@/lib/search";
import type { AuditEntry } from "@/lib/types";

export const Route = createFileRoute("/admin/audit")({
  validateSearch: listSearchSchema,
  staticData: { title: "Audit Log" },
  beforeLoad: ({ location }) => ensurePermission("audit", "read", location),
  component: AuditPage,
});

function AuditPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [action, setAction] = useState("");
  const [resource, setResource] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const debouncedAction = useDebounced(action);
  const debouncedResource = useDebounced(resource);

  const limit = 50;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    action: debouncedAction || undefined,
    resource: debouncedResource || undefined,
  };
  const table = useTableView<AuditEntry>("audit", params, search.view);

  const setSearch = (patch: Partial<ListSearch>) => {
    void navigate({ to: "/admin/audit", search: { ...search, ...patch } });
  };

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Audit log</h1>
        <p className="text-sm text-muted-foreground">
          Who changed what, when, and from where. Records are immutable.
        </p>
      </div>
      <ExplainerBanner
        title="Audit — the paper trail"
        howTo={[
          "Every mutation through the admin console writes one record: actor, action, resource, IP, request id.",
          'Rows with a diff expand to show the {"before","after"} snapshot.',
          "Filter by action (e.g. role.updated) or resource type to investigate a change.",
        ]}
      >
        Audit writes are fire-and-forget: a logging failure never blocks the underlying request, and
        the log itself is append-only — retention is the only deletion path. The same pagination
        switchers as every other table drive GET /api/audit; the feed loads 50 rows per step.
      </ExplainerBanner>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-56">
          <Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            className="pl-8"
            placeholder="Filter by action (e.g. user.)"
            value={action}
            onChange={(e) => setAction(e.target.value)}
          />
        </div>
        <Input
          className="w-48"
          placeholder="Resource type (user, role…)"
          value={resource}
          onChange={(e) => setResource(e.target.value)}
        />
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          onClick={() => void queryClient.invalidateQueries({ queryKey: ["audit"] })}
        >
          Refresh
        </Button>
      </div>

      <div className="space-y-3">
        <div className="overflow-hidden rounded-lg border bg-card">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">When</th>
                <th className="px-3 py-2 font-medium">Actor</th>
                <th className="px-3 py-2 font-medium">Action</th>
                <th className="px-3 py-2 font-medium">Resource</th>
                <th className="px-3 py-2 font-medium">IP</th>
              </tr>
            </thead>
            <tbody>
              {table.isPending ? (
                Array.from({ length: 5 }, (_unused, index) => (
                  <tr key={index}>
                    <td colSpan={5} className="px-3 py-2">
                      <Skeleton className="h-4 w-full max-w-80" />
                    </td>
                  </tr>
                ))
              ) : table.rows.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-muted-foreground">
                    No audit records match.
                  </td>
                </tr>
              ) : (
                table.rows.map((entry) => (
                  <AuditRow
                    key={entry.id}
                    entry={entry}
                    expanded={expanded === entry.id}
                    onToggle={() => setExpanded((current) => (current === entry.id ? null : entry.id))}
                  />
                ))
              )}
            </tbody>
          </table>
          {search.view === "infinite" && table.hasMore && !table.isFetchingMore ? (
            <InfiniteSentinel onLoadMore={table.loadMore} />
          ) : null}
        </div>
        <PaginationBar
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
        />
      </div>
    </div>
  );
}

function AuditRow({
  entry,
  expanded,
  onToggle,
}: {
  entry: AuditEntry;
  expanded: boolean;
  onToggle: () => void;
}) {
  const hasDiff = Boolean(entry.diff) && entry.diff !== "{}";
  return (
    <>
      <tr className="border-t hover:bg-muted/30">
        <td className="px-3 py-2 whitespace-nowrap text-muted-foreground">
          {format(new Date(entry.created_at), "MMM d, HH:mm:ss")}
        </td>
        <td className="px-3 py-2">{entry.actor_email || entry.actor_id || "—"}</td>
        <td className="px-3 py-2 font-mono text-xs">{entry.action}</td>
        <td className="px-3 py-2">
          {entry.resource_type}
          {entry.resource_id ? (
            <span className="text-muted-foreground"> · {entry.resource_id.slice(0, 10)}</span>
          ) : null}
        </td>
        <td className="px-3 py-2 text-muted-foreground">{entry.ip || "—"}</td>
      </tr>
      {expanded && (
        <tr className="border-t bg-muted/20">
          <td colSpan={5} className="px-3 py-2">
            {hasDiff ? (
              <pre className="max-h-56 overflow-auto rounded bg-muted p-2 font-mono text-xs">
                {JSON.stringify(JSON.parse(entry.diff ?? "{}"), null, 2)}
              </pre>
            ) : (
              <p className="text-xs text-muted-foreground">No payload recorded for this action.</p>
            )}
            {entry.request_id && (
              <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                request {entry.request_id}
              </p>
            )}
          </td>
        </tr>
      )}
      {hasDiff && (
        <tr className="-mt-2">
          <td colSpan={5} className="px-3 pb-1">
            <button
              type="button"
              onClick={onToggle}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
            >
              {expanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
              {expanded ? "Hide diff" : "Show diff"}
            </button>
          </td>
        </tr>
      )}
    </>
  );
}
