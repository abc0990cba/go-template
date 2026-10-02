import { BookOpen } from "lucide-react";
import { useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/shared/components/ui/alert";
import { ChevronDown } from "lucide-react";
import { cn } from "@/shared/lib/utils";

interface ExplainerBannerProps {
  title: string;
  children: React.ReactNode;
  /** Extra bullets rendered as a "How to use" list. */
  howTo?: string[];
  defaultOpen?: boolean;
}

/**
 * ExplainerBanner is the per-page teaching panel: what the feature is, how
 * to use it, and what happens under the hood. Every authenticated page
 * mounts one under its header so the app explains itself.
 */
export function ExplainerBanner({
  title,
  children,
  howTo,
  defaultOpen = false,
}: ExplainerBannerProps) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Alert className="border-dashed">
      <BookOpen className="size-4" />
      <AlertTitle className="flex items-center justify-between gap-2">
        {title}
        <button
          type="button"
          onClick={() => setOpen((previous) => !previous)}
          className="ml-auto inline-flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground"
          aria-expanded={open}
        >
          {open ? "Hide" : "Explain"}
          <ChevronDown className={cn("size-3.5 transition-transform", open && "rotate-180")} />
        </button>
      </AlertTitle>
      <AlertDescription>
        <div className={cn("space-y-2 text-sm", !open && "sr-only")}>
          <div className="text-foreground/90">{children}</div>
          {howTo && howTo.length > 0 ? (
            <ul className="list-disc space-y-1 pl-4">
              {howTo.map((step) => (
                <li key={step}>{step}</li>
              ))}
            </ul>
          ) : null}
        </div>
      </AlertDescription>
    </Alert>
  );
}
