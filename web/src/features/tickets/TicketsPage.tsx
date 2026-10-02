import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { Plus, RefreshCw } from "lucide-react";
import { DataTable } from "@/shared/components/data-table/data-table";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { useLive } from "@/features/live";
import { useTableView } from "@/shared/hooks/use-paged-list";
import { useCan } from "@/shared/hooks/use-permissions";
import type { ColumnDef } from "@tanstack/react-table";
import { CreateTicketDialog } from "./CreateTicketDialog";
import { listTickets, ticketCounts } from "./api";
import type { TicketsSearch } from "./schemas";
import { TicketStatusBadge, ticketLabel } from "./TicketStatusBadge";
import type { Ticket } from "./types";
import { Button } from "@/shared/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/shared/components/ui/tabs";

export function TicketsPage({ search }: { search: TicketsSearch }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const can = useCan();
  const [createOpen, setCreateOpen] = useState(false);
  useLive();

  const canReadAll = can("tickets", "read_all");
  const scope: "my" | "all" = search.scope === "all" && canReadAll ? "all" : "my";

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    q: search.q,
    status: search.status,
    scope,
  };
  const table = useTableView<Ticket>("tickets", listTickets, params, search.view);
  const counts = useQuery({
    queryKey: ["tickets", "counts", scope],
    queryFn: () => ticketCounts(scope),
  });

  const setSearch = (patch: Partial<TicketsSearch>) => {
    void navigate({ to: "/tickets", search: { ...search, ...patch } });
  };

  const columns: ColumnDef<Ticket>[] = [
    {
      accessorKey: "number",
      header: "#",
      cell: ({ row }) => (
        <span className="font-mono text-xs">{ticketLabel(row.original.number)}</span>
      ),
    },
    {
      accessorKey: "title",
      header: "Title",
      cell: ({ row }) => (
        <a
          href={`/tickets/${row.original.id}`}
          className="font-medium underline-offset-4 hover:underline"
        >
          {row.original.title}
        </a>
      ),
    },
    { accessorKey: "type_key", header: "Type" },
    {
      accessorKey: "priority",
      header: "Priority",
      cell: ({ row }) => <span className="capitalize">{row.original.priority}</span>,
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <TicketStatusBadge status={row.original.status} />,
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, HH:mm")}
        </span>
      ),
    },
  ];

  const chip = (key: string) => (
    <span key={key} className="rounded-md border px-2 py-1 text-xs text-muted-foreground">
      {key.replace("_", " ")}:{" "}
      <span className="font-semibold text-foreground">{counts.data?.[key] ?? 0}</span>
    </span>
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">My Tickets</h1>
          <p className="text-sm text-muted-foreground">
            File requests, follow their approval progress, deliver the result — or withdraw your own
            ticket.
          </p>
        </div>
        <div className="ml-auto">
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus /> New ticket
          </Button>
        </div>
      </div>

      <ExplainerBanner
        title="The requester side of the flow"
        howTo={[
          "Create a draft from a ticket type — the type carries the approval chain.",
          "Submit it to move it in review; the current step's approver role gets notified.",
          "Withdraw whenever you like — even after approval; deliver the result to finish, or rework after a rejection.",
        ]}
      >
        Your view is scoped to tickets you created
        {canReadAll ? ", with an “All (org)” tab for org-wide reading." : "."} The Approvals page is
        the other side of the flow: tickets waiting on your roles appear there, not here.
      </ExplainerBanner>

      <div className="flex flex-wrap items-center gap-2">
        {canReadAll && (
          <Tabs
            value={scope}
            onValueChange={(value) =>
              setSearch({ scope: value as "my" | "all", page: 1, after: undefined })
            }
          >
            <TabsList>
              <TabsTrigger value="my">My tickets</TabsTrigger>
              <TabsTrigger value="all">All (org)</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
        {!canReadAll && chip("draft")}
        {["draft", "in_review", "approved", "completed", "rejected", "withdrawn"].map(chip)}
      </div>

      <DataTable
        columns={columns}
        data={table.rows}
        isPending={table.isPending}
        isPlaceholderData={table.isPlaceholderData}
        emptyState="No tickets yet. File the first one with “New ticket”."
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
              <SelectTrigger className="w-44" aria-label="Filter by status">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="draft">Draft</SelectItem>
                <SelectItem value="in_review">In review</SelectItem>
                <SelectItem value="approved">Approved</SelectItem>
                <SelectItem value="completed">Completed</SelectItem>
                <SelectItem value="rejected">Rejected by approver</SelectItem>
                <SelectItem value="withdrawn">Withdrawn by requester</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void queryClient.invalidateQueries({ queryKey: ["tickets"] })}
            >
              <RefreshCw /> Refresh
            </Button>
          </>
        }
      />

      <CreateTicketDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}
