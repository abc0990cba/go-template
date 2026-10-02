import { Info } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/components/ui/tooltip";
import { cn } from "@/shared/lib/utils";
import type { PaginationMode } from "@/shared/api/pagination";

const MODES: { value: PaginationMode; label: string; hint: string }[] = [
  {
    value: "offset",
    label: "Offset",
    hint: "Classic LIMIT/OFFSET paging: easy page numbers, but the database counts past skipped rows, and rows shifting between requests can duplicate or skip entries.",
  },
  {
    value: "cursor",
    label: "Cursor",
    hint: "Opaque continuation tokens: the server returns the key of the last row and the next page resumes right after it. Stable under inserts, no page numbers.",
  },
  {
    value: "keyset",
    label: "Keyset",
    hint: "WHERE (created_at, id) < last row — the predicate walks the composite index directly, which keeps p95 latency flat no matter how deep you page. The repo's k6 targets test exactly this.",
  },
];

interface ModeSwitcherProps {
  mode: PaginationMode;
  onModeChange: (mode: PaginationMode) => void;
  disabled?: boolean;
}

/** ModeSwitcher is the three-way control that selects the pagination strategy. */
export function ModeSwitcher({ mode, onModeChange, disabled = false }: ModeSwitcherProps) {
  return (
    <div className="flex items-center gap-2">
      <div className="inline-flex rounded-lg border bg-muted p-0.5" aria-label="Pagination mode">
        {MODES.map((candidate) => (
          <button
            key={candidate.value}
            type="button"
            disabled={disabled}
            onClick={() => onModeChange(candidate.value)}
            className={cn(
              "rounded-md px-3 py-1 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50",
              mode === candidate.value
                ? "bg-background shadow-sm text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
            aria-pressed={mode === candidate.value}
          >
            {candidate.label}
          </button>
        ))}
      </div>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="What do these modes mean?"
            className="text-muted-foreground hover:text-foreground"
          >
            <Info className="size-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-90 space-y-1.5">
          {MODES.map((candidate) => (
            <p key={candidate.value}>
              <span className="font-semibold">{candidate.label}:</span> {candidate.hint}
            </p>
          ))}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

export { MODES as PAGINATION_MODES };
