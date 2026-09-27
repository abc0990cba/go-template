import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { format } from "date-fns";
import { ArrowLeft } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";

export const Route = createFileRoute("/admin/users/$userId")({
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
        </CardContent>
      </Card>

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
