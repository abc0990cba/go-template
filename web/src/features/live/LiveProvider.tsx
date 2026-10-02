import { useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type { AnalyticsSummary, EventRecord, Job, Me, Notification } from "@/lib/types";

type StreamMessage =
  | { type: "connected"; payload: { at: string } }
  | { type: "analytics.event"; payload: EventRecord }
  | { type: "job.updated"; payload: Job }
  | { type: "notification.created"; payload: Notification }
  | { type: "snapshot"; payload: { summary: AnalyticsSummary; jobs: Job[] } };

export type ConnectionStatus = "connecting" | "live" | "offline";

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
  const dirty = useRef({ analytics: false, jobs: false });

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
    }, 2000);
    return () => clearInterval(flush);
  }, [queryClient]);

  useEffect(() => {
    const source = new EventSource("/api/events/stream");
    const onOpen = () => setStatus("live");
    const onError = () => setStatus("offline");
    const onMessage = (message: MessageEvent<string>) => {
      let event: StreamMessage;
      try {
        event = JSON.parse(message.data) as StreamMessage;
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
    };
    source.addEventListener("open", onOpen);
    source.addEventListener("error", onError);
    source.addEventListener("message", onMessage);

    return () => {
      source.removeEventListener("open", onOpen);
      source.removeEventListener("error", onError);
      source.removeEventListener("message", onMessage);
      source.close();
    };
  }, [queryClient]);

  return <LiveContext.Provider value={{ status, feed, jobs }}>{children}</LiveContext.Provider>;
}

export function useLive(): LiveContextValue {
  return useContext(LiveContext);
}
