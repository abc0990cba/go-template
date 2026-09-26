import { Badge } from "@/components/ui/badge";
import type { JobStatus } from "@/lib/types";

const STATUS_VARIANT: Record<
  JobStatus | "done" | "failed",
  "default" | "secondary" | "destructive" | "outline"
> = {
  queued: "outline",
  running: "secondary",
  done: "default",
  failed: "destructive",
};

/** StatusBadge renders a job/report status with a consistent palette. */
export function StatusBadge({ status }: { status: string }) {
  const variant = STATUS_VARIANT[status as JobStatus] ?? "outline";
  return <Badge variant={variant}>{status}</Badge>;
}
