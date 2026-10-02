import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate, Link } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, ArrowLeft, Check, CheckCheck, Undo2, X } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { useCan, useMe } from "@/shared/hooks/use-permissions";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  addTicketComment,
  deleteTicket,
  getTicket,
  ticketAction,
  ticketComments,
  ticketEvents,
} from "./api";
import { TicketStatusBadge, ticketLabel } from "./TicketStatusBadge";
import { ApprovalStep, CommentForm, RejectDialog, TerminalOutcome } from "./TicketDetailParts";

export function TicketDetailPage({ ticketId }: { ticketId: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const can = useCan();
  const me = useMe();
  const [rejectOpen, setRejectOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const detail = useSuspenseQuery({
    queryKey: ["tickets", "single", ticketId],
    queryFn: () => getTicket(ticketId),
    retry: false,
  });
  const comments = useQuery({
    queryKey: ["tickets", "comments", ticketId],
    queryFn: () => ticketComments(ticketId),
    enabled: detail.isSuccess,
  });
  const events = useQuery({
    queryKey: ["tickets", "events", ticketId],
    queryFn: () => ticketEvents(ticketId),
    enabled: detail.isSuccess,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["tickets"] });
    void queryClient.invalidateQueries({ queryKey: ["approvals"] });
  };

  const act = useMutation({
    mutationFn: ({
      action,
      comment,
    }: {
      action: "submit" | "approve" | "reject" | "withdraw" | "complete" | "reopen";
      comment?: string;
    }) => ticketAction(ticketId, action, comment ?? ""),
    onSuccess: (ticket, { action }) => {
      toast.success(`Ticket ${ticketLabel(ticket.number)}: ${action} done`);
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const comment = useMutation({
    mutationFn: (body: string) => addTicketComment(ticketId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["tickets", "comments", ticketId] });
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const remove = useMutation({
    mutationFn: () => deleteTicket(ticketId),
    onSuccess: () => {
      toast.success("Draft deleted");
      invalidate();
      void navigate({ to: "/tickets" });
    },
    onError: (error) => toast.error(error.message),
  });

  if (detail.isError) {
    return (
      <Alert variant="destructive">
        <AlertTriangle />
        <AlertTitle>Ticket unavailable</AlertTitle>
        <AlertDescription>
          It does not exist, or you are not one of the people involved.{" "}
          <Link to="/tickets" className="underline">
            Back to my tickets.
          </Link>
        </AlertDescription>
      </Alert>
    );
  }

  const { ticket, approvals } = detail.data;
  const mine = ticket.created_by === me.id;
  const current = approvals[ticket.current_step];
  // Mirrors the server guard: the step's role decides, admins may bypass.
  // The server re-checks everything; these buttons are a convenience.
  const canDecideCurrent =
    ticket.status === "in_review" &&
    current?.status === "pending" &&
    can("tickets", "approve") &&
    (me.roles.includes(current.approver_role) ||
      me.roles.includes("admin") ||
      me.roles.includes("superadmin"));

  return (
    <div className="space-y-4">
      <Button variant="ghost" size="sm" className="-ml-2" asChild>
        <Link to="/tickets">
          <ArrowLeft /> My tickets
        </Link>
      </Button>

      <div className="flex flex-wrap items-start gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
            <span className="font-mono text-sm text-muted-foreground">
              {ticketLabel(ticket.number)}
            </span>
            {ticket.title}
          </h1>
          <p className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
            <TicketStatusBadge status={ticket.status} />
            <span className="capitalize">{ticket.priority} priority</span>
            <span>·</span>
            <span>type: {ticket.type_key}</span>
            <span>·</span>
            <span>filed {format(new Date(ticket.created_at), "MMM d, yyyy HH:mm")}</span>
          </p>
        </div>
        <div className="ml-auto flex flex-wrap gap-2">
          {mine && ticket.status === "draft" && (
            <>
              <Button
                size="sm"
                onClick={() => act.mutate({ action: "submit" })}
                disabled={act.isPending}
              >
                <CheckCheck /> Submit for approval
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="text-destructive"
                onClick={() => setDeleteOpen(true)}
              >
                Delete
              </Button>
            </>
          )}
          {mine &&
            (ticket.status === "draft" ||
              ticket.status === "in_review" ||
              ticket.status === "approved") && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => act.mutate({ action: "withdraw" })}
                disabled={act.isPending}
              >
                <Undo2 /> Withdraw
              </Button>
            )}
          {mine && ticket.status === "approved" && (
            <Button
              size="sm"
              onClick={() => act.mutate({ action: "complete" })}
              disabled={act.isPending}
            >
              <CheckCheck /> Deliver result
            </Button>
          )}
          {mine && ticket.status === "rejected" && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => act.mutate({ action: "reopen" })}
              disabled={act.isPending}
            >
              <Undo2 /> Rework
            </Button>
          )}
          {canDecideCurrent && (
            <>
              <Button
                size="sm"
                onClick={() => act.mutate({ action: "approve" })}
                disabled={act.isPending}
              >
                <Check /> Approve
              </Button>
              <Button size="sm" variant="destructive" onClick={() => setRejectOpen(true)}>
                <X /> Reject
              </Button>
            </>
          )}
        </div>
      </div>

      <TerminalOutcome
        status={ticket.status}
        decidedBy={ticket.decided_by}
        decidedAt={ticket.decided_at}
        approvals={approvals}
      />

      <ExplainerBanner
        title="Who can act, and when"
        howTo={[
          "The requester owns the flow: submit, withdraw, deliver the result, rework after a rejection.",
          "Approvers act only on the current step of the chain, in order — and rejections require a comment.",
          "Withdrawn and rejected are different outcomes: the first closes the ticket by its author, the second by an approver.",
        ]}
      >
        Actions you cannot perform are hidden rather than disabled — the same guards run again
        server-side, so the UI is a convenience, never the boundary.
      </ExplainerBanner>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <p className="whitespace-pre-wrap">
              {ticket.description || "No description provided."}
            </p>
            <p className="text-xs text-muted-foreground">
              Requester <span className="font-mono">{ticket.created_by}</span>
              {ticket.decided_by && (
                <>
                  {" · "}decided by <span className="font-mono">{ticket.decided_by}</span>
                  {ticket.decided_at &&
                    ` at ${format(new Date(ticket.decided_at), "MMM d, HH:mm")}`}
                </>
              )}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Approval chain</CardTitle>
            <CardDescription>
              Steps run in order; later steps wait for earlier ones.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {approvals.length === 0 && (
              <p className="text-sm text-muted-foreground">
                The chain appears once the ticket is submitted.
              </p>
            )}
            {approvals.map((step) => (
              <ApprovalStep
                key={step.id}
                step={step}
                current={step.step_index === ticket.current_step}
              />
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Timeline</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {(events.data ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">No events yet.</p>
            )}
            {(events.data ?? []).map((event) => (
              <div key={event.id} className="flex items-start gap-2 text-sm">
                <span className="mt-1 size-1.5 shrink-0 rounded-full bg-muted-foreground" />
                <div>
                  <span className="font-medium">{event.action.replace("_", " ")}</span>
                  {event.detail && <span className="text-muted-foreground"> — {event.detail}</span>}
                  <p className="text-xs text-muted-foreground">
                    <span className="font-mono">{event.actor_id}</span> ·{" "}
                    {format(new Date(event.created_at), "MMM d, HH:mm")}
                  </p>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Comments</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-2">
              {(comments.data ?? []).length === 0 && (
                <p className="text-sm text-muted-foreground">No comments yet.</p>
              )}
              {(comments.data ?? []).map((entry) => (
                <div key={entry.id} className="rounded-md border p-2 text-sm">
                  <p className="whitespace-pre-wrap">{entry.body}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    <span className="font-mono">{entry.author_id}</span> ·{" "}
                    {format(new Date(entry.created_at), "MMM d, HH:mm")}
                  </p>
                </div>
              ))}
            </div>
            <CommentForm onSubmit={(body) => comment.mutate(body)} pending={comment.isPending} />
          </CardContent>
        </Card>
      </div>

      <RejectDialog
        open={rejectOpen}
        pending={act.isPending}
        onOpenChange={setRejectOpen}
        onConfirm={(text) => {
          act.mutate({ action: "reject", comment: text });
          setRejectOpen(false);
        }}
      />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this draft?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the ticket with its comments and timeline. There is no undo.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                remove.mutate();
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
