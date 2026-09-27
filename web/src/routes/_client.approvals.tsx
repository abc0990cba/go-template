import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { Check, RefreshCw, X } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { TicketStatusBadge, ticketLabel } from "@/components/ticket-status-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { listSearchSchema, type ListSearch } from "@/lib/search";
import type { Ticket, TicketDecision } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";

export const Route = createFileRoute("/_client/approvals")({
  validateSearch: listSearchSchema,
  staticData: { title: "Approvals" },
  component: ApprovalsPage,
});

/**
 * ApprovalsPage is the approver's interface: an inbox of tickets whose
 * current step waits on one of the caller's roles, plus the history of
 * decisions already made. It is deliberately a separate page from My
 * Tickets — filing and deciding are different jobs.
 */
function ApprovalsPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<"inbox" | "decided">("inbox");

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    q: search.q,
  };
  const inboxQuery = useQuery({
    queryKey: ["approvals", "inbox", params],
    queryFn: () => api.approvals(params),
    placeholderData: (previous) => previous,
  });
  const decided = useQuery({
    queryKey: ["approvals", "decided"],
    queryFn: () => api.decidedApprovals(50),
  });

  const [decision, setDecision] = useState<{ ticket: Ticket; action: "approve" | "reject" } | null>(null);

  const act = useMutation({
    mutationFn: ({ id, action, comment }: { id: string; action: "approve" | "reject"; comment: string }) =>
      api.ticketAction(id, action, comment),
    onSuccess: (ticket, { action }) => {
      toast.success(`Ticket ${ticketLabel(ticket.number)} ${action === "approve" ? "approved" : "rejected"}`);
      setDecision(null);
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      void queryClient.invalidateQueries({ queryKey: ["tickets"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const setSearch = (patch: Partial<ListSearch>) => {
    void navigate({ to: "/approvals", search: { ...search, ...patch } });
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
      header: "Filed",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, HH:mm")}
        </span>
      ),
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      cell: ({ row }) => (
        <div className="flex justify-end gap-1">
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1"
            onClick={() => setDecision({ ticket: row.original, action: "approve" })}
          >
            <Check /> Approve
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1 text-destructive"
            onClick={() => setDecision({ ticket: row.original, action: "reject" })}
          >
            <X /> Reject
          </Button>
        </div>
      ),
    },
  ];

  const decisionColumns: ColumnDef<TicketDecision>[] = [
    {
      accessorKey: "ticket",
      header: "Ticket",
      cell: ({ row }) => (
        <a href={`/tickets/${row.original.ticket.id}`} className="underline-offset-4 hover:underline">
          <span className="font-mono text-xs">{ticketLabel(row.original.ticket.number)}</span>{" "}
          {row.original.ticket.title}
        </a>
      ),
    },
    {
      accessorKey: "approval",
      header: "Step",
      cell: ({ row }) => (
        <span>
          {row.original.approval.name}{" "}
          <Badge variant="outline" className="ml-1">
            {row.original.approval.approver_role}
          </Badge>
        </span>
      ),
    },
    {
      accessorFn: (row) => row.approval.status,
      header: "Decision",
      cell: ({ row }) => (
        <Badge
          variant="outline"
          className={
            row.original.approval.status === "approved"
              ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
              : "bg-destructive/10 text-destructive"
          }
        >
          {row.original.approval.status}
        </Badge>
      ),
    },
    {
      accessorFn: (row) => row.approval.comment ?? "",
      header: "Comment",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">{row.original.approval.comment || "—"}</span>
      ),
    },
    {
      accessorFn: (row) => row.approval.decided_at ?? "",
      header: "Decided",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {row.original.approval.decided_at
            ? format(new Date(row.original.approval.decided_at), "MMM d, HH:mm")
            : "—"}
        </span>
      ),
    },
  ];

  const activeInbox = inboxQuery.data?.data;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Approvals</h1>
        <p className="text-sm text-muted-foreground">
          Tickets whose current approval step waits on one of your roles. Approving advances the
          chain; rejecting ends it — with a comment.
        </p>
      </div>

      <ExplainerBanner
        title="The approver side of the flow"
        howTo={[
          "The inbox lists only tickets waiting on your roles at their current step — order matters in multi-step chains.",
          "Rejections require a comment; the requester sees it and can rework the ticket.",
          "Everything you decided lands in the “My decisions” tab.",
        ]}
      >
        This queue is role-scoped server-side: another approver with different roles sees a
        different inbox. Requesters never see this page unless they also hold an approver role.
      </ExplainerBanner>

      <Tabs value={tab} onValueChange={(value) => setTab(value as "inbox" | "decided")}>
        <TabsList>
          <TabsTrigger value="inbox">
            Waiting for you {activeInbox?.length ? `(${activeInbox.length})` : ""}
          </TabsTrigger>
          <TabsTrigger value="decided">My decisions</TabsTrigger>
        </TabsList>
      </Tabs>

      {tab === "inbox" ? (
        <DataTable
          columns={columns}
          data={activeInbox}
          isPending={inboxQuery.isPending}
          isPlaceholderData={inboxQuery.isPlaceholderData}
          emptyState="Nothing waits on your roles right now."
          pagination={{
            mode: search.mode,
            page: search.page,
            hasMore: (inboxQuery.data?.next_cursor ?? "") !== "",
            onFirstPage: () => setSearch({ page: 1, after: undefined }),
            onPreviousPage: () => setSearch({ page: Math.max(1, search.page - 1) }),
            onNextPage: () =>
              search.mode === "offset"
                ? setSearch({ page: search.page + 1 })
                : setSearch({ page: search.page + 1, after: inboxQuery.data?.next_cursor }),
            onModeChange: (mode) => setSearch({ mode, page: 1, after: undefined }),
          }}
          toolbar={
            <Button
              variant="outline"
              size="sm"
              onClick={() => void queryClient.invalidateQueries({ queryKey: ["approvals"] })}
            >
              <RefreshCw /> Refresh
            </Button>
          }
        />
      ) : (
        <DataTable
          columns={decisionColumns}
          data={decided.data?.data}
          isPending={decided.isPending}
          isPlaceholderData={false}
          emptyState="You have not decided on any approval step yet."
        />
      )}

      <DecisionDialog
        decision={decision}
        pending={act.isPending}
        onOpenChange={(open) => !open && setDecision(null)}
        onConfirm={(comment) => {
          if (decision) act.mutate({ id: decision.ticket.id, action: decision.action, comment });
        }}
      />
    </div>
  );
}

function DecisionDialog({
  decision,
  pending,
  onOpenChange,
  onConfirm,
}: {
  decision: { ticket: Ticket; action: "approve" | "reject" } | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (comment: string) => void;
}) {
  const [comment, setComment] = useState("");
  const rejecting = decision?.action === "reject";
  const confirm = () => {
    onConfirm(comment.trim());
    setComment("");
  };
  return (
    <Dialog open={decision !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {decision ? `${rejecting ? "Reject" : "Approve"} ${ticketLabel(decision.ticket.number)}` : ""}
          </DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">{decision?.ticket.title}</p>
        <div className="grid gap-1.5">
          <Label htmlFor="decision-comment">
            Comment {rejecting ? "(required)" : "(optional)"}
          </Label>
          <Textarea
            id="decision-comment"
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            rows={3}
            placeholder={rejecting ? "Why is this rejected? The requester sees it." : "Add context for the requester…"}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant={rejecting ? "destructive" : "default"}
            onClick={confirm}
            disabled={pending || (rejecting && !comment.trim())}
          >
            {rejecting ? "Reject ticket" : "Approve ticket"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
