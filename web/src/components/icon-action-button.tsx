import type { LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

/**
 * IconActionButton is a small icon button with a tooltip explaining what it
 * does — row actions must never make the user guess an icon. Set
 * `destructive` for state-destroying actions.
 */
export function IconActionButton({
  icon: Icon,
  tooltip,
  onClick,
  destructive = false,
  disabled = false,
  label,
  children,
}: {
  icon: LucideIcon;
  tooltip: string;
  onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void;
  destructive?: boolean;
  disabled?: boolean;
  label: string;
  children?: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          // 36px touch target on phones (28px on desktop): row actions stay
          // thumb-accurate without inflating desktop row density.
          className={`size-9 md:size-7 ${destructive ? "text-destructive" : ""}`}
          onClick={onClick}
          disabled={disabled}
          aria-label={label}
        >
          {children ?? <Icon />}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">
        <p>{tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
