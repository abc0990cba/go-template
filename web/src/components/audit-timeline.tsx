import { useInfiniteQuery } from "@tanstack/react-query";
import { format, formatDistanceToNow } from "date-fns";
import {
  Building2,
  ChevronDown,
  ChevronUp,
  KeyRound,
  KeySquare,
  Loader2,
  LogIn,
  LogOut,
  Pencil,
  Radio,
  ScrollText,
  Settings,
  ShieldCheck,
  ShieldOff,
  Ticket,
  Trash2,
  UserPlus,
  VenetianMask,
  Webhook,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { AuditEntry } from "@/lib/types";
import { cn } from "@/shared/lib/utils";
import { UserAvatar } from "@/components/user-avatar";

/** How the timeline presents one audit action. */
interface ActionStyle {
  icon: LucideIcon;
  /** Bubble + text tones; plain Tailwind so the scanner sees them. */
  bubble: string;
  label: string;
}

const ACTION_STYLES: Record<string, ActionStyle> = {
  "user.created": {
    icon: UserPlus,
    bubble: "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400",
    label: "Account created",
  },
  "user.registered": {
    icon: UserPlus,
    bubble: "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400",
    label: "Self-registered",
  },
  "user.updated": {
    icon: Pencil,
    bubble: "bg-sky-500/12 text-sky-600 dark:text-sky-400",
    label: "Profile updated",
  },
  "user.deleted": {
    icon: Trash2,
    bubble: "bg-red-500/12 text-red-600 dark:text-red-400",
    label: "Account deleted",
  },
  "user.logged_in": {
    icon: LogIn,
    bubble: "bg-teal-500/12 text-teal-600 dark:text-teal-400",
    label: "Signed in",
  },
  "user.logged_out": {
    icon: LogOut,
    bubble: "bg-slate-500/12 text-slate-600 dark:text-slate-400",
    label: "Signed out",
  },
  "user.password_reset": {
    icon: KeyRound,
    bubble: "bg-amber-500/12 text-amber-600 dark:text-amber-400",
    label: "Password reset",
  },
  "user.role_assigned": {
    icon: ShieldCheck,
    bubble: "bg-violet-500/12 text-violet-600 dark:text-violet-400",
    label: "Role assigned",
  },
  "user.role_revoked": {
    icon: ShieldOff,
    bubble: "bg-orange-500/12 text-orange-600 dark:text-orange-400",
    label: "Role revoked",
  },
  "impersonation.started": {
    icon: VenetianMask,
    bubble: "bg-amber-500/12 text-amber-600 dark:text-amber-400",
    label: "Impersonation started",
  },
  "impersonation.stopped": {
    icon: VenetianMask,
    bubble: "bg-slate-500/12 text-slate-600 dark:text-slate-400",
    label: "Impersonation stopped",
  },
  "streams.demo_published": {
    icon: Radio,
    bubble: "bg-cyan-500/12 text-cyan-600 dark:text-cyan-400",
    label: "Stream event published",
  },
  "ticket.created": {
    icon: Ticket,
    bubble: "bg-blue-500/12 text-blue-600 dark:text-blue-400",
    label: "Ticket created",
  },
  "role.created": {
    icon: KeySquare,
    bubble: "bg-violet-500/12 text-violet-600 dark:text-violet-400",
    label: "Role created",
  },
  "org.created": {
    icon: Building2,
    bubble: "bg-indigo-500/12 text-indigo-600 dark:text-indigo-400",
    label: "Organization created",
  },
  "org.member_added": {
    icon: Building2,
    bubble: "bg-indigo-500/12 text-indigo-600 dark:text-indigo-400",
    label: "Member added to org",
  },
  "org.member_removed": {
    icon: Building2,
    bubble: "bg-indigo-500/12 text-indigo-600 dark:text-indigo-400",
    label: "Member removed from org",
  },
  "webhook.created": {
    icon: Webhook,
    bubble: "bg-fuchsia-500/12 text-fuchsia-600 dark:text-fuchsia-400",
    label: "Webhook created",
  },
  "settings.updated": {
    icon: Settings,
    bubble: "bg-slate-500/12 text-slate-600 dark:text-slate-400",
    label: "Setting changed",
  },
};

const FALLBACK_STYLE: ActionStyle = {
  icon: ScrollText,
  bubble: "bg-muted text-muted-foreground",
  label: "",
};

function actionStyle(action: string): ActionStyle {
  const exact = ACTION_STYLES[action];
  if (exact) return exact;
  if (action.startsWith("ticket."))
    return { ...FALLBACK_STYLE, icon: Ticket, label: titleize(action) };
  if (action.startsWith("role."))
    return { ...FALLBACK_STYLE, icon: KeySquare, label: titleize(action) };
  if (action.startsWith("org."))
    return { ...FALLBACK_STYLE, icon: Building2, label: titleize(action) };
  if (action.startsWith("webhook."))
    return { ...FALLBACK_STYLE, icon: Webhook, label: titleize(action) };
  if (action.startsWith("api_key."))
    return { ...FALLBACK_STYLE, icon: KeyRound, label: titleize(action) };
  if (action.startsWith("streams."))
    return { ...FALLBACK_STYLE, icon: Radio, label: titleize(action) };
  if (action.startsWith("policy."))
    return { ...FALLBACK_STYLE, icon: ShieldCheck, label: titleize(action) };
  return { ...FALLBACK_STYLE, label: titleize(action) };
}

/** "user.role_assigned" → "role assigned". */
function titleize(action: string) {
  return action.split(".").slice(1).join(" ").replaceAll("_", " ");
}

export type ActivityFilter = "all" | "about" | "by";

const FILTERS: { key: ActivityFilter; label: string }[] = [
  { key: "all", label: "Everything" },
  { key: "about", label: "About this user" },
  { key: "by", label: "By this user" },
];

const PAGE_SIZE = 25;

/**
 * AuditTimeline is the user card's history feed: every audit record where
 * the person is the actor or the subject, newest first. The feed scrolls
 * INSIDE a fixed-height panel — the page below never grows no matter how
 * much history loads — and the sentinel observes that panel, so the next
 * page loads as the reader approaches the bottom of the feed itself.
 */
export function AuditTimeline({ userId, className }: { userId: string; className?: string }) {
  const [filter, setFilter] = useState<ActivityFilter>("all");
  const [expanded, setExpanded] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const feed = useInfiniteQuery({
    queryKey: ["users", userId, "activity"],
    queryFn: ({ pageParam }) => api.userActivity(userId, PAGE_SIZE, Number(pageParam)),
    initialPageParam: 0,
    getNextPageParam: (lastPage): number | undefined => {
      if (!lastPage.next_cursor) return undefined;
      return Number(lastPage.next_cursor);
    },
  });

  const rows = (feed.data?.pages ?? []).flatMap((page) => page.data ?? []);
  const filtered = rows.filter((entry) => {
    if (filter === "about") return entry.resource_type === "user" && entry.resource_id === userId;
    if (filter === "by") return entry.actor_id === userId;
    return true;
  });

  const canLoadMore = feed.hasNextPage && !feed.isFetchingNextPage;
  const loadMore = () => void feed.fetchNextPage();

  // The sentinel lives inside the scroll panel; observing with the panel
  // as the intersection root triggers loading exactly when its bottom
  // scrolls into view (and never from page-level scrolling).
  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinelRef.current;
    const root = scrollRef.current;
    if (!node || !root || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { root, rootMargin: "160px 0px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [feed.hasNextPage, feed.isFetchingNextPage]);

  return (
    <div className={cn("flex min-h-0 flex-col", className)}>
      {/* min-w-0 + truncate on the buttons: their labels must never drive
          the panel wider than the screen — the whole page's grid track
          would follow the widest min-content. */}
      <div className="flex flex-wrap items-center gap-1 rounded-lg bg-muted p-1">
        {FILTERS.map((candidate) => (
          <button
            key={candidate.key}
            type="button"
            onClick={() => setFilter(candidate.key)}
            className={cn(
              "min-h-9 min-w-0 flex-1 truncate rounded-md px-3 text-xs font-medium text-muted-foreground transition-colors sm:flex-none sm:min-w-32",
              filter === candidate.key && "bg-background text-foreground shadow-xs",
            )}
          >
            {candidate.label}
          </button>
        ))}
      </div>

      <div
        ref={scrollRef}
        className="mt-2 min-h-0 flex-1 overflow-y-auto overscroll-contain rounded-lg border"
      >
        {feed.isPending ? (
          <div className="space-y-3 p-3">
            {Array.from({ length: 4 }, (_unused, index) => (
              <div key={index} className="flex gap-3">
                <Skeleton className="size-9 shrink-0 rounded-full" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3.5 w-32" />
                  <Skeleton className="h-3 w-48 max-w-full" />
                </div>
              </div>
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <p className="p-6 text-center text-sm text-muted-foreground">
            {rows.length === 0
              ? "No history yet — changes to this account and actions by this user will appear here."
              : "Nothing matches this filter in the loaded history."}
          </p>
        ) : (
          <ol className="relative px-3 py-2">
            {filtered.map((entry, index) => (
              <TimelineRow
                key={entry.id}
                entry={entry}
                last={index === filtered.length - 1}
                selfActor={entry.actor_id === userId}
                expanded={expanded === entry.id}
                onToggle={() => setExpanded((current) => (current === entry.id ? null : entry.id))}
              />
            ))}
            {feed.isFetchingNextPage ? (
              <li className="flex items-center justify-center gap-2 py-3 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" /> Loading more…
              </li>
            ) : null}
            {canLoadMore ? <div ref={sentinelRef} aria-hidden className="h-px" /> : null}
          </ol>
        )}
      </div>

      <div className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>
          {rows.length} record{rows.length === 1 ? "" : "s"} loaded
          {feed.hasNextPage ? " · scrolling loads older entries" : ""}
        </span>
        {feed.hasNextPage ? (
          <Button
            size="sm"
            variant="outline"
            className="h-8"
            onClick={loadMore}
            disabled={feed.isFetchingNextPage}
          >
            {feed.isFetchingNextPage ? <Loader2 className="animate-spin" /> : null} Load more
          </Button>
        ) : null}
      </div>
      {feed.isError ? (
        <p className="mt-1 text-xs text-destructive">{(feed.error as Error).message}</p>
      ) : null}
    </div>
  );
}

function TimelineRow({
  entry,
  last,
  selfActor,
  expanded,
  onToggle,
}: {
  entry: AuditEntry;
  last: boolean;
  selfActor: boolean;
  expanded: boolean;
  onToggle: () => void;
}) {
  const style = actionStyle(entry.action);
  const Icon = style.icon;
  const hasDiff = Boolean(entry.diff) && entry.diff !== "{}";

  return (
    <li className="relative flex gap-3 pb-4 last:pb-1">
      {/* Rail: bubble + connecting line, the classic activity-feed spine. */}
      <div className="flex flex-col items-center">
        <span
          className={cn(
            "z-10 flex size-9 shrink-0 items-center justify-center rounded-full ring-4 ring-card",
            style.bubble,
          )}
        >
          <Icon className="size-4" />
        </span>
        {!last ? <span className="w-px flex-1 bg-border" aria-hidden /> : null}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <p className="text-sm font-medium">{style.label || entry.action}</p>
          <time
            className="text-xs text-muted-foreground"
            dateTime={entry.created_at}
            title={format(new Date(entry.created_at), "MMM d, yyyy HH:mm:ss")}
          >
            {formatDistanceToNow(new Date(entry.created_at), { addSuffix: true })}
          </time>
        </div>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">
          {selfActor ? "This user" : entry.actor_email || "system"} · {entry.resource_type}
          {entry.resource_id ? ` · ${entry.resource_id.slice(0, 12)}` : ""}
          {entry.ip ? ` · ${entry.ip}` : ""}
        </p>

        {hasDiff ? (
          <div className="mt-1.5">
            <button
              type="button"
              onClick={onToggle}
              aria-expanded={expanded}
              className="inline-flex min-h-8 items-center gap-1 rounded-md text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              {expanded ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
              {expanded ? "Hide changes" : "Show changes"}
            </button>
            {expanded ? (
              <>
                <DiffView diff={entry.diff ?? "{}"} />
                {entry.request_id ? (
                  <p className="mt-1.5 font-mono text-[10px] text-muted-foreground">
                    request {entry.request_id}
                  </p>
                ) : null}
              </>
            ) : null}
          </div>
        ) : null}

        {!selfActor && entry.actor_id ? (
          <div className="mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground">
            <UserAvatar
              name={entry.actor_email}
              email={entry.actor_email}
              className="size-4"
              fallbackClassName="text-[8px]"
            />
            <span>by {entry.actor_email || entry.actor_id.slice(0, 12)}</span>
          </div>
        ) : null}
      </div>
    </li>
  );
}

/**
 * DiffView renders the recorded {"before","after"} snapshot: changed-field
 * chips on top, then the two snapshots side by side (stacked on phones).
 */
function DiffView({ diff }: { diff: string }) {
  let before: unknown;
  let after: unknown;
  try {
    const parsed = JSON.parse(diff) as { before?: unknown; after?: unknown };
    before = parsed.before;
    after = parsed.after;
  } catch {
    return (
      <pre className="mt-1 overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs">{diff}</pre>
    );
  }

  const changed = changedKeys(before, after);

  return (
    <div className="mt-1 space-y-2">
      {changed.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {changed.map((key) => (
            <span
              key={key}
              className="rounded-full bg-amber-500/12 px-2 py-0.5 font-mono text-[10px] font-medium text-amber-700 dark:text-amber-400"
            >
              {key}
            </span>
          ))}
        </div>
      ) : null}
      <div className="grid gap-2 sm:grid-cols-2">
        <Snapshot title="Before" value={before} tone="border-red-500/25 bg-red-500/[0.04]" />
        <Snapshot title="After" value={after} tone="border-emerald-500/25 bg-emerald-500/[0.04]" />
      </div>
    </div>
  );
}

function Snapshot({ title, value, tone }: { title: string; value: unknown; tone: string }) {
  return (
    <div className={cn("overflow-hidden rounded-md border", tone)}>
      <p className="border-b bg-background/60 px-2 py-1 text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
        {title}
      </p>
      <pre className="max-h-44 overflow-auto p-2 font-mono text-[11px] leading-relaxed">
        {value == null ? "—" : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

/** Keys whose JSON representation differs between the two snapshots. */
function changedKeys(before: unknown, after: unknown): string[] {
  if (typeof before !== "object" || typeof after !== "object" || !before || !after) return [];
  const keys = new Set([...Object.keys(before as object), ...Object.keys(after as object)]);
  return [...keys].filter((key) => {
    const a = (before as Record<string, unknown>)[key];
    const b = (after as Record<string, unknown>)[key];
    return JSON.stringify(a) !== JSON.stringify(b);
  });
}
