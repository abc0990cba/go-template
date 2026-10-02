import { useInfiniteQuery } from "@tanstack/react-query";

import { Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/shared/components/ui/button";
import { Skeleton } from "@/shared/components/ui/skeleton";
import { userActivity } from "@/features/admin-users";

import { cn } from "@/shared/lib/utils";

/** How the timeline presents one audit action. */

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
    queryFn: ({ pageParam }) => userActivity(userId, PAGE_SIZE, Number(pageParam)),
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

import { TimelineRow, type ActivityFilter } from "./AuditTimeline-parts";
