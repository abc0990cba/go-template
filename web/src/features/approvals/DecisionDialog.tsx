import { useState } from "react";
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
import { ticketLabel } from "@/features/tickets";
import type { Ticket } from "@/features/tickets";

export function DecisionDialog({
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
            {decision
              ? `${rejecting ? "Reject" : "Approve"} ${ticketLabel(decision.ticket.number)}`
              : ""}
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
            placeholder={
              rejecting
                ? "Why is this rejected? The requester sees it."
                : "Add context for the requester…"
            }
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
