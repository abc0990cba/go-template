import { useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { connectRealtime, type ConnectionStatus } from "./transport";
import type { EventRecord } from "./types";
import type { AnalyticsSummary } from "@/features/admin-insights/types";

import type { Job } from "@/features/admin-ops";
import type { Me } from "@/shared/types/core";
import type { Notification } from "@/features/notifications";
import type { Ticket } from "@/features/tickets";

type StreamMessage =
  | { type: "connected"; payload: { at: string } }
  | { type: "analytics.event"; payload: EventRecord }
  | { type: "job.updated"; payload: Job }
  | { type: "ticket.updated"; payload: Ticket }
  | { type: "notification.created"; payload: Notification }
  | { type: "snapshot"; payload: { summary: AnalyticsSummary; jobs: Job[] } };

export type { ConnectionStatus } from "./transport";

interface LiveContextValue {
  status: ConnectionStatus;
  /** Newest-first ring buffer of the analytics event trail. */
  feed: EventRecord[];
  jobs: Job[];
}

const LiveContext = createContext<LiveContextValue>({ status: "connecting", feed: [], jobs: [] });

const FEED_LIMIT = 60;

/**
 * LiveProvider owns the single Server-Sent Events connection of the app.
 * Incoming messages update the TanStack Query cache (throttled) and the
 * shared feed that dashboards render. EventSource reconnects on its own;
 * we only track the status for the UI badge.
 */
export function LiveProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<ConnectionStatus>("connecting");
  const [feed, setFeed] = useState<EventRecord[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const dirty = useRef({ analytics: false, jobs: false, tickets: false });

  useEffect(() => {
    const flush = setInterval(() => {
      if (dirty.current.analytics) {
        dirty.current.analytics = false;
        void queryClient.invalidateQueries({ queryKey: ["analytics"] });
      }
      if (dirty.current.jobs) {
        dirty.current.jobs = false;
        void queryClient.invalidateQueries({ queryKey: ["jobs"] });
        void queryClient.invalidateQueries({ queryKey: ["reports"] });
      }
      if (dirty.current.tickets) {
        dirty.current.tickets = false;
        void queryClient.invalidateQueries({ queryKey: ["tickets"] });
      }
    }, 2000);
    return () => clearInterval(flush);
  }, [queryClient]);

  useEffect(() => {
    // WebSocket first, SSE fallback — the frames are identical JSON.
    return connectRealtime({
      onStatus: (next) => setStatus(next),
      onMessage: (data) => {
        let event: StreamMessage;
        try {
          event = JSON.parse(data) as StreamMessage;
        } catch {
          return;
        }
        switch (event.type) {
          case "connected": {
            setStatus("live");
            break;
          }
          case "analytics.event": {
            dirty.current.analytics = true;
            setFeed((previous) => [event.payload, ...previous].slice(0, FEED_LIMIT));
            break;
          }
          case "notification.created": {
            const me = queryClient.getQueryData<Me>(["me"]);
            if (me && event.payload.user_id === me.id) {
              toast.info(event.payload.title, {
                description: event.payload.body || undefined,
              });
              void queryClient.invalidateQueries({ queryKey: ["notifications"] });
            }
            break;
          }
          case "ticket.updated": {
            // Org-scoped on the wire; a ticket changed in one of my orgs.
            dirty.current.tickets = true;
            break;
          }
          case "job.updated": {
            dirty.current.jobs = true;
            setJobs((previous) =>
              [event.payload, ...previous.filter((j) => j.id !== event.payload.id)].slice(
                0,
                FEED_LIMIT,
              ),
            );
            queryClient.setQueryData<Job>(["jobs", "single", event.payload.id], event.payload);
            break;
          }
          case "snapshot": {
            setStatus("live");
            setJobs((previous) => {
              const updated = new Map(previous.map((j) => [j.id, j]));
              for (const job of event.payload.jobs) {
                if (!updated.has(job.id)) updated.set(job.id, job);
              }
              return [...updated.values()]
                .toSorted((a, b) => b.created_at.localeCompare(a.created_at))
                .slice(0, FEED_LIMIT);
            });
            dirty.current.analytics = true;
            queryClient.setQueryData<AnalyticsSummary>(
              ["analytics", "summary"],
              event.payload.summary,
            );
            break;
          }
        }
      },
    });
  }, [queryClient]);

  return <LiveContext.Provider value={{ status, feed, jobs }}>{children}</LiveContext.Provider>;
}

export function useLive(): LiveContextValue {
  return useContext(LiveContext);
}
