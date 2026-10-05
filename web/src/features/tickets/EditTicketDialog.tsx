import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/shared/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import { Input } from "@/shared/components/ui/input";
import { Label } from "@/shared/components/ui/label";
import { Textarea } from "@/shared/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import { isStaleWrite } from "@/shared/api/core";

import { getTicketWithETag, updateTicket } from "./api";
import type { Ticket, TicketPriority } from "./types";

/**
 * EditTicketDialog edits a draft's title, details, and priority behind
 * optimistic locking: the ETag is fetched fresh when the dialog opens and
 * echoed as If-Match, so a concurrent change answers 412 and the dialog
 * reloads instead of clobbering the other editor.
 */
export function EditTicketDialog({
  ticket,
  open,
  onOpenChange,
  onSaved,
}: {
  ticket: Ticket;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState(ticket.title);
  const [description, setDescription] = useState(ticket.description);
  const [priority, setPriority] = useState<TicketPriority>(ticket.priority);
  const [etag, setEtag] = useState("");

  // A fresh tag per opening: the draft list may be minutes behind.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    getTicketWithETag(ticket.id)
      .then(({ data, etag: current }) => {
        if (cancelled) return;
        setEtag(current);
        setTitle(data.ticket.title);
        setDescription(data.ticket.description);
        setPriority(data.ticket.priority);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [open, ticket.id]);

  const update = useMutation({
    mutationFn: () => updateTicket(ticket.id, title, description, priority, etag),
    onSuccess: () => {
      toast.success("Ticket updated");
      onOpenChange(false);
      onSaved();
    },
    onError: (error) => {
      if (isStaleWrite(error)) {
        toast.error(
          "The ticket was changed by someone else — the latest version has been loaded; please review and save again.",
        );
        void queryClient.invalidateQueries({ queryKey: ["tickets"] });
        getTicketWithETag(ticket.id).then(({ data, etag: fresh }) => {
          setEtag(fresh);
          setTitle(data.ticket.title);
          setDescription(data.ticket.description);
          setPriority(data.ticket.priority);
        });
        return;
      }
      toast.error(error.message);
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit draft</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="edit-ticket-title">Title</Label>
            <Input
              id="edit-ticket-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="edit-ticket-description">Details</Label>
            <Textarea
              id="edit-ticket-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="edit-ticket-priority">Priority</Label>
            <Select
              value={priority}
              onValueChange={(value) => setPriority(value as TicketPriority)}
            >
              <SelectTrigger id="edit-ticket-priority" aria-label="Priority">
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
          <Button onClick={() => update.mutate()} disabled={!title.trim() || update.isPending}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
