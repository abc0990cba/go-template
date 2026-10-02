import { Badge } from "@/components/ui/badge";
import { useI18n, type TKey } from "@/shared/lib/i18n";
import type { TicketStatus } from "./types";
import { cn } from "@/shared/lib/utils";

const STATUS_CLASS: Record<TicketStatus, string> = {
  draft: "bg-muted text-muted-foreground",
  in_review: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  approved: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  completed: "bg-primary/10 text-primary",
  // The two terminal outcomes stay visually distinct on purpose: rejected
  // (by an approver) reads as an error, withdrawn (by the requester) as a
  // neutral early exit.
  rejected: "bg-destructive/10 text-destructive",
  withdrawn: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
};

/**
 * TicketStatusBadge renders a ticket lifecycle status. "Rejected by
 * approver" and "Withdrawn by requester" get separate labels and colors in
 * both UI languages — collapsing them into one "closed" state is exactly
 * the ambiguity this screen must avoid.
 */
export function TicketStatusBadge({ status, className }: { status: string; className?: string }) {
  const { t } = useI18n();
  const label = t(`ticket.status.${status}` as TKey) ?? status;
  return (
    <Badge variant="outline" className={cn(STATUS_CLASS[status as TicketStatus], className)}>
      {label}
    </Badge>
  );
}

/** ticketLabel renders the human-readable reference, e.g. TCK-0042. */
export function ticketLabel(number: number): string {
  return `TCK-${String(number).padStart(4, "0")}`;
}
