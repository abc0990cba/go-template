import { ArrowDownToLine, Info, List } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { ViewMode } from "@/lib/types";

const VIEWS: { value: ViewMode; label: string; hint: string }[] = [
  {
    value: "paged",
    label: "Pages",
    hint: "Classic controls: jump forward or back a page at a time. Works with every strategy.",
  },
  {
    value: "infinite",
    label: "Infinite",
    hint: "Continuous feed: the next page appends automatically as you scroll toward the bottom.",
  },
];

interface ViewSwitcherProps {
  view: ViewMode;
  onViewChange: (view: ViewMode) => void;
  disabled?: boolean;
}

/** ViewSwitcher toggles a table between page controls and infinite scroll. */
export function ViewSwitcher({ view, onViewChange, disabled = false }: ViewSwitcherProps) {
  return (
    <div className="flex items-center gap-1.5">
      <div className="inline-flex rounded-lg border bg-muted p-0.5" aria-label="Table view">
        {VIEWS.map((candidate) => (
          <button
            key={candidate.value}
            type="button"
            disabled={disabled}
            onClick={() => onViewChange(candidate.value)}
            className={cn(
              "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50",
              view === candidate.value
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
            aria-pressed={view === candidate.value}
          >
            {candidate.value === "paged" ? (
              <List className="size-3.5" />
            ) : (
              <ArrowDownToLine className="size-3.5" />
            )}
            {candidate.label}
          </button>
        ))}
      </div>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="What do these views mean?"
            className="text-muted-foreground hover:text-foreground"
          >
            <Info className="size-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-80 space-y-1.5">
          {VIEWS.map((candidate) => (
            <p key={candidate.value}>
              <span className="font-semibold">{candidate.label}:</span> {candidate.hint}
            </p>
          ))}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
