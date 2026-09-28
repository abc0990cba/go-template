import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { format } from "date-fns";
import { ArrowLeft, Send } from "lucide-react";
import { toast } from "sonner";
import { ExplainerBanner } from "@/components/explainer-banner";
import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";

export const Route = createFileRoute("/admin/users_/$userId")({
  staticData: { title: "User" },
  component: UserDetailPage,
});

function UserDetailPage() {
  const { userId } = Route.useParams();

  const user = useSuspenseQuery({
    queryKey: ["users", "single", userId],
    queryFn: () => api.getUser(userId),
  });
  const profiles = useQuery({
    queryKey: ["enrichment-profiles", "user", userId],
    queryFn: () => api.listEnrichmentProfiles({ mode: "offset", limit: 25, user_id: userId }),
  });

  if (user.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>User not found</AlertTitle>
        <AlertDescription>
          The account does not exist (or was deleted).{" "}
          <Link to="/admin/users" className="underline">
            Back to users.
          </Link>
        </AlertDescription>
      </Alert>
    );
  }

  const u = user.data;
  const queryClient = useQueryClient();
  const enrichDemo = useMutation({
    mutationFn: ({ source }: { source: "kafka" | "nats" }) => api.triggerEnrichDemo(userId, source),
    onSuccess: (result) => {
      toast.success(
        `Published ${result.source} event ${result.event_id.slice(0, 18)}… — the worker applies it in a moment`,
      );
      void queryClient.invalidateQueries({ queryKey: ["users", "single", userId] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <div className="space-y-4">
      <Button variant="ghost" size="sm" className="-ml-2" asChild>
        <Link to="/admin/users">
          <ArrowLeft /> All users
        </Link>
      </Button>

      <Card>
        <CardHeader className="flex flex-row items-center gap-4">
          <Avatar className="size-12">
            <AvatarFallback className="text-lg">
              {(u.name || u.email).charAt(0).toUpperCase()}
            </AvatarFallback>
          </Avatar>
          <div>
            <CardTitle>{u.name || "—"}</CardTitle>
            <CardDescription className="font-mono">{u.email}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="grid gap-2 text-sm sm:grid-cols-2">
          <p className="text-muted-foreground">
            Created{" "}
            <span className="text-foreground">
              {format(new Date(u.created_at), "MMM d, yyyy HH:mm")}
            </span>
          </p>
          <p className="text-muted-foreground">
            Updated{" "}
            <span className="text-foreground">
              {format(new Date(u.updated_at), "MMM d, yyyy HH:mm")}
            </span>
          </p>
          <p className="text-muted-foreground sm:col-span-2">
            Id <span className="font-mono text-foreground">{u.id}</span>
          </p>
          {u.external_ref ? (
            <p className="text-muted-foreground sm:col-span-2">
              External ref <span className="font-mono text-foreground">{u.external_ref}</span>{" "}
              <span className="text-xs">(correlation id for the enrichment streams)</span>
            </p>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <EnrichmentCard
          title="Profile (Kafka)"
          description="Owned fields merged from the user-profile.v1 topic."
          payload={u.enrich_profile}
          externalRef={u.external_ref}
          onTest={() => enrichDemo.mutate({ source: "kafka" })}
          testing={enrichDemo.isPending && enrichDemo.variables?.source === "kafka"}
        />
        <EnrichmentCard
          title="Contact (NATS)"
          description="Owned fields merged from the users.contact.v1 subject."
          payload={u.enrich_contact}
          externalRef={u.external_ref}
          onTest={() => enrichDemo.mutate({ source: "nats" })}
          testing={enrichDemo.isPending && enrichDemo.variables?.source === "nats"}
        />
      </div>

      <ExplainerBanner title="Enrichment profiles for this user" defaultOpen>
        Enrichment jobs fetch external profiles from randomuser.me and store them under a user id.
        This section asks <code>GET /api/enrichment-profiles?user_id=…</code> — a filter the backend
        applies before pagination.
      </ExplainerBanner>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Fetched profiles</CardTitle>
          <CardDescription>Raw payloads exactly as stored, newest first.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {profiles.isPending ? (
            <Skeleton className="h-16 w-full" />
          ) : (profiles.data?.data.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">No enrichment profiles for this user.</p>
          ) : (
            profiles.data?.data.map((profile, index) => (
              <div key={index} className="rounded-md border p-2">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span className="font-mono">{profile.source}</span>
                  <span>{format(new Date(profile.created_at), "MMM d, HH:mm")}</span>
                </div>
                <pre className="mt-1 overflow-x-auto text-xs">{profile.payload}</pre>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <StatusBadge status="done" /> example: statuses across the app share one badge component.
      </div>
    </div>
  );
}

/**
 * EnrichmentCard renders one stream's owned JSONB payload, pretty-printed,
 * with a one-click test trigger: it publishes a real event into that stream
 * and the worker applies it through the normal exactly-once pipeline.
 */
function EnrichmentCard({
  title,
  description,
  payload,
  externalRef,
  onTest,
  testing,
}: {
  title: string;
  description: string;
  payload?: string;
  externalRef?: string;
  onTest: () => void;
  testing: boolean;
}) {
  let pretty: string | null = null;
  if (payload) {
    try {
      pretty = JSON.stringify(JSON.parse(payload), null, 2);
    } catch {
      pretty = payload;
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">
            {externalRef ? (
              <>
                external_ref <code className="font-mono">{externalRef}</code>
              </>
            ) : (
              <span>
                No external_ref — edit the user and set one, or the streams cannot find it.
              </span>
            )}
          </p>
          <Button size="sm" variant="outline" disabled={testing || !externalRef} onClick={onTest}>
            <Send /> {testing ? "Publishing…" : "Send test event"}
          </Button>
        </div>
        {pretty ? (
          <pre className="max-h-56 overflow-auto rounded-md bg-muted p-2 font-mono text-xs">
            {pretty}
          </pre>
        ) : (
          <p className="text-sm text-muted-foreground">
            No enrichment yet. Click “Send test event” to publish into this stream and watch the
            card fill in.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
