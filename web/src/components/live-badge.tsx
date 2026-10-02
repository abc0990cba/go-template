import { cn } from "@/shared/lib/utils";
import { useLive, type ConnectionStatus } from "@/hooks/use-event-stream";

const LABEL: Record<ConnectionStatus, string> = {
  connecting: "Connecting…",
  live: "Live",
  offline: "Reconnecting…",
};

/** LiveBadge shows the SSE connection state; tooltips explain the stream. */
export function LiveBadge({ className }: { className?: string }) {
  const { status } = useLive();
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium",
        status === "live" &&
          "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
        status === "connecting" &&
          "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400",
        status === "offline" && "border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400",
        className,
      )}
      title="Server-Sent Events stream: analytics events, job updates, and snapshots pushed from the backend."
    >
      <span className="relative flex size-2">
        <span
          className={cn(
            "absolute inline-flex h-full w-full rounded-full opacity-60",
            status === "live" && "animate-ping bg-emerald-500",
            status === "connecting" && "bg-amber-500",
            status === "offline" && "bg-red-500",
          )}
        />
        <span
          className={cn(
            "relative inline-flex size-2 rounded-full",
            status === "live" && "bg-emerald-500",
            status === "connecting" && "bg-amber-500",
            status === "offline" && "bg-red-500",
          )}
        />
      </span>
      {/* On phones the dot carries the state; the label needs the space. */}
      <span className="hidden sm:inline">{LABEL[status]}</span>
    </span>
  );
}
