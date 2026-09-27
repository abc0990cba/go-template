import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { Plus, RefreshCw } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { TicketStatusBadge, ticketLabel } from "@/components/ticket-status-badge";
import { useLive } from "@/hooks/use-event-stream";
import { usePagedList } from "@/hooks/use-paged-list";
import { useCan } from "@/hooks/use-permissions";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { listSearchSchema, type ListSearch } from "@/lib/search";
import type { Ticket } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";
import { z } from "zod";

const ticketsSearchSchema = listSearchSchema.extend({
  scope: z.enum(["my", "all"]).catch("my").default("my"),
});

export const Route = createFileRoute("/_client/tickets")({
  validateSearch: ticketsSearchSchema,
  staticData: { title: "My Tickets" },
  component: TicketsPage,
});

function TicketsPage() {
  const search = Route.useSearch();
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
  const tickets = usePagedList<Ticket>("tickets", params);
  const counts = useQuery({
    queryKey: ["tickets", "counts", scope],
    queryFn: () => api.ticketCounts(scope),
  });

  const setSearch = (patch: Partial<ListSearch> & { scope?: "my" | "all" }) => {
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
      {key.replace("_", " ")}: <span className="font-semibold text-foreground">{counts.data?.[key] ?? 0}</span>
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
          <Tabs value={scope} onValueChange={(value) => setSearch({ scope: value as "my" | "all", page: 1, after: undefined })}>
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
        data={tickets.data?.data}
        isPending={tickets.isPending}
        isPlaceholderData={tickets.isPlaceholderData}
        emptyState="No tickets yet. File the first one with “New ticket”."
        pagination={{
          mode: search.mode,
          page: search.page,
          hasMore: (tickets.data?.next_cursor ?? "") !== "",
          onFirstPage: () => setSearch({ page: 1, after: undefined }),
          onPreviousPage: () => setSearch({ page: Math.max(1, search.page - 1) }),
          onNextPage: () =>
            search.mode === "offset"
              ? setSearch({ page: search.page + 1 })
              : setSearch({ page: search.page + 1, after: tickets.data?.next_cursor }),
          onModeChange: (mode) => setSearch({ mode, page: 1, after: undefined }),
        }}
        toolbar={
          <>
            <Select
              value={search.status ?? "all"}
              onValueChange={(value) =>
                setSearch({ status: value === "all" ? undefined : value, page: 1, after: undefined })
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

function CreateTicketDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const types = useQuery({ queryKey: ["ticket-types"], queryFn: () => api.ticketTypes() });
  const [typeId, setTypeId] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("medium");

  const create = useMutation({
    mutationFn: () => api.createTicket(typeId, title, description, priority),
    onSuccess: (ticket) => {
      toast.success(`Ticket ${ticketLabel(ticket.number)} created as a draft`);
      void queryClient.invalidateQueries({ queryKey: ["tickets"] });
      onOpenChange(false);
      setTitle("");
      setDescription("");
      setTypeId("");
      setPriority("medium");
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New ticket</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="ticket-type">Type</Label>
            <Select value={typeId} onValueChange={setTypeId}>
              <SelectTrigger id="ticket-type" aria-label="Ticket type">
                <SelectValue placeholder="Pick a type" />
              </SelectTrigger>
              <SelectContent>
                {(types.data ?? [])
                  .filter((typ) => typ.active)
                  .map((typ) => (
                    <SelectItem key={typ.id} value={typ.id}>
                      {typ.name} — {typ.steps.length} step{typ.steps.length > 1 ? "s" : ""}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ticket-title">Title</Label>
            <Input
              id="ticket-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="What do you need?"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ticket-description">Details</Label>
            <Textarea
              id="ticket-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ticket-priority">Priority</Label>
            <Select value={priority} onValueChange={setPriority}>
              <SelectTrigger id="ticket-priority" aria-label="Priority">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="low">Low</SelectItem>
                <SelectItem value="medium">Medium</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="urgent">Urgent</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => create.mutate()} disabled={!typeId || !title.trim() || create.isPending}>
            Create draft
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
