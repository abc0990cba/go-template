import { format, formatDistanceToNow } from "date-fns";
import { useState } from "react";
import { Ban, Check, CircleDashed, SkipForward, Undo2, X } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { TicketApproval, TicketStatus } from "./types";

/**
 * TerminalOutcome names who ended the ticket and why — the explicit
 * difference between "withdrawn by the requester" and "rejected by an
 * approver" that the status badges only abbreviate.
 */
export function TerminalOutcome({
  status,
  decidedBy,
  decidedAt,
  approvals,
}: {
  status: TicketStatus;
  decidedBy?: string;
  decidedAt: string | null;
  approvals: TicketApproval[];
}) {
  if (status !== "rejected" && status !== "withdrawn") return null;
  const rejected = status === "rejected";
  const rejection = approvals.find((step) => step.status === "rejected");
  return (
    <Alert
      variant={rejected ? "destructive" : undefined}
      className={rejected ? "" : "border-amber-500/50 text-amber-600 dark:text-amber-400"}
    >
      {rejected ? <Ban /> : <Undo2 />}
      <AlertTitle>
        {rejected ? "Rejected by the approver" : "Withdrawn by the requester"}
      </AlertTitle>
      <AlertDescription>
        {rejected ? (
          <>
            <span className="font-mono">{decidedBy}</span> rejected this ticket at the step “
            {rejection?.name ?? "review"}”
            {decidedAt && ` on ${format(new Date(decidedAt), "MMM d, yyyy HH:mm")}`}. The requester
            can rework it and submit again.
            {rejection?.comment && (
              <span className="mt-1 block font-medium text-foreground">“{rejection.comment}”</span>
            )}
          </>
        ) : (
          <>
            <span className="font-mono">{decidedBy}</span> withdrew this ticket
            {decidedAt && ` on ${format(new Date(decidedAt), "MMM d, yyyy HH:mm")}`}. No approver
            declined it — the author simply closed it. Withdrawn tickets stay closed.
          </>
        )}
      </AlertDescription>
    </Alert>
  );
}

export function ApprovalStep({ step, current }: { step: TicketApproval; current: boolean }) {
  const icon =
    step.status === "approved" ? (
      <Check className="text-emerald-500" />
    ) : step.status === "rejected" ? (
      <X className="text-destructive" />
    ) : step.status === "skipped" ? (
      <SkipForward className="text-muted-foreground" />
    ) : current ? (
      <CircleDashed className="animate-spin text-blue-500 [animation-duration:3s]" />
    ) : (
      <CircleDashed className="text-muted-foreground" />
    );
  return (
    <div
      className={`flex items-start gap-2 rounded-md border p-2 text-sm ${current ? "border-blue-500/50" : ""}`}
    >
      <span className="mt-0.5">{icon}</span>
      <div className="flex-1">
        <p className="font-medium">
          {step.name}{" "}
          <Badge variant="outline" className="ml-1 font-normal">
            role: {step.approver_role}
          </Badge>
          {current && step.status === "pending" && (
            <Badge
              className="ml-1 bg-blue-500/15 text-blue-600 dark:text-blue-400"
              variant="outline"
            >
              waiting now
            </Badge>
          )}
        </p>
        {step.decided_at && (
          <p className="text-xs text-muted-foreground">
            decided {formatDistanceToNow(new Date(step.decided_at))} ago by{" "}
            <span className="font-mono">{step.decided_by}</span>
          </p>
        )}
        {step.comment && <p className="mt-1 text-xs">“{step.comment}”</p>}
      </div>
    </div>
  );
}

export function CommentForm({
  onSubmit,
  pending,
}: {
  onSubmit: (body: string) => void;
  pending: boolean;
}) {
  const [body, setBody] = useState("");
  return (
    <div className="grid gap-1.5">
      <Label htmlFor="new-comment">Add a comment</Label>
      <Textarea
        id="new-comment"
        rows={2}
        value={body}
        onChange={(event) => setBody(event.target.value)}
        placeholder="Visible to everyone involved in the ticket…"
      />
      <Button
        size="sm"
        className="justify-self-start"
        disabled={!body.trim() || pending}
        onClick={() => {
          onSubmit(body.trim());
          setBody("");
        }}
      >
        Comment
      </Button>
    </div>
  );
}

export function RejectDialog({
  open,
  pending,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (comment: string) => void;
}) {
  const [text, setText] = useState("");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reject this ticket?</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          The rejection ends the flow. Later steps are skipped and the requester sees your comment.
        </p>
        <div className="grid gap-1.5">
          <Label htmlFor="reject-comment">Comment (required)</Label>
          <Textarea
            id="reject-comment"
            rows={3}
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="Why is this rejected?"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={!text.trim() || pending}
            onClick={() => {
              onConfirm(text.trim());
              setText("");
            }}
          >
            Reject ticket
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
