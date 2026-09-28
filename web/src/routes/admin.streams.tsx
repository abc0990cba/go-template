import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { RotateCcw, RefreshCw } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ensurePermission } from "@/lib/auth";
import { api } from "@/lib/api";
import type { DeadLetter, OutboxEvent } from "@/lib/types";

export const Route = createFileRoute("/admin/streams")({
  staticData: { title: "Streams" },
  beforeLoad: ({ location }) => ensurePermission("streams", "read", location),
  component: StreamsPage,
});

function StreamsPage() {
  const [tab, setTab] = useState<"outbox" | "dead">("outbox");
  const queryClient = useQueryClient();
  const [replayError, setReplayError] = useState("");

  const outbox = useQuery({
    queryKey: ["streams", "outbox"],
    queryFn: () => api.listOutbox({ mode: "offset", limit: 50, status: "pending" }),
  });
  const dead = useQuery({
    queryKey: ["streams", "dead-letters"],
    queryFn: () => api.listDeadLetters({ mode: "offset", limit: 50, pending: true }),
  });

  const replay = useMutation({
    mutationFn: (id: string) => api.replayDeadLetter(id),
    onSuccess: () => {
      setReplayError("");
      void queryClient.invalidateQueries({ queryKey: ["streams"] });
    },
    onError: (error: Error) => {
      setReplayError(error.message);
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Streams</h1>
          <p className="text-sm text-muted-foreground">
            The transactional outbox and the dead-letter queue behind the Kafka + NATS enrichment.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void queryClient.invalidateQueries({ queryKey: ["streams"] })}
        >
          <RefreshCw /> Refresh
        </Button>
      </div>

      <ExplainerBanner
        title="Streams — exactly-once by construction"
        howTo={[
          "Outbox: domain events written in the same transaction as the change; the worker relay publishes them and marks them processed.",
          "Dead letters: poison or unknown-user messages parked instead of blocking their stream.",
          "Replay re-runs a parked message through the live handler path — after the user exists, unknown_user letters land.",
        ]}
      >
        Every enrichment event applies exactly once: the apply transaction dedupes by (source,
        event_id), drops stale sequence numbers, merges only the fields its source owns, and commits
        an outbox row atomically. This page is the operational window onto that machinery; the same
        data is on GET /api/streams/outbox and /api/streams/dead-letters.
      </ExplainerBanner>

      <div className="flex gap-2">
        <Button
          variant={tab === "outbox" ? "default" : "outline"}
          size="sm"
          onClick={() => setTab("outbox")}
        >
          Outbox {outbox.data?.pending != null ? `(${outbox.data.pending} pending)` : ""}
        </Button>
        <Button
          variant={tab === "dead" ? "default" : "outline"}
          size="sm"
          onClick={() => setTab("dead")}
        >
          Dead letters {dead.data?.pending != null ? `(${dead.data.pending})` : ""}
        </Button>
      </div>

      {replayError ? (
        <Alert variant="destructive">
          <AlertTitle>Replay failed</AlertTitle>
          <AlertDescription>{replayError}</AlertDescription>
        </Alert>
      ) : null}

      {tab === "outbox" ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Pending outbox events</CardTitle>
            <CardDescription>
              Written with the change, waiting for the relay to publish.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {outbox.isPending ? (
              <Skeleton className="h-16 w-full" />
            ) : (outbox.data?.data?.length ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">
                Nothing pending — the relay has published everything.
              </p>
            ) : (
              outbox.data?.data?.map((event) => <OutboxRow key={event.id} event={event} />)
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Parked messages</CardTitle>
            <CardDescription>
              Poison or unprocessable events. Replay re-runs them through the live handler.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {dead.isPending ? (
              <Skeleton className="h-16 w-full" />
            ) : (dead.data?.data?.length ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">
                No dead letters. Publish a poison message with <code>make demo-dlq</code> to see one
                here.
              </p>
            ) : (
              dead.data?.data?.map((letter) => (
                <DeadLetterRow
                  key={letter.id}
                  letter={letter}
                  onReplay={() => replay.mutate(letter.id)}
                  replaying={replay.isPending && replay.variables === letter.id}
                />
              ))
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function OutboxRow({ event }: { event: OutboxEvent }) {
  return (
    <div className="rounded-md border p-2">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-mono">{event.event_type}</span>
        <span>
          attempts {event.attempts} · {format(new Date(event.created_at), "MMM d, HH:mm:ss")}
        </span>
      </div>
      <pre className="mt-1 overflow-x-auto text-xs">{event.payload}</pre>
    </div>
  );
}

function DeadLetterRow({
  letter,
  onReplay,
  replaying,
}: {
  letter: DeadLetter;
  onReplay: () => void;
  replaying: boolean;
}) {
  let pretty = letter.payload;
  try {
    pretty = JSON.stringify(JSON.parse(letter.payload), null, 2);
  } catch {
    // keep the raw payload for malformed letters
  }
  return (
    <div className="rounded-md border p-2">
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>
          <span className="font-mono text-foreground">{letter.source}</span> · {letter.reason} ·{" "}
          {letter.topic || "—"} · attempts {letter.attempts} ·{" "}
          {format(new Date(letter.created_at), "MMM d, HH:mm:ss")}
        </span>
        <Button variant="outline" size="sm" disabled={replaying} onClick={onReplay}>
          <RotateCcw /> {replaying ? "Replaying…" : "Replay"}
        </Button>
      </div>
      <pre className="mt-1 max-h-32 overflow-auto text-xs">{pretty}</pre>
    </div>
  );
}
