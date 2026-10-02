import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { format } from "date-fns";
import { ArrowLeft, CalendarClock, Loader2, Pencil, VenetianMask } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { AuditTimeline } from "@/features/admin-insights";
import { ExplainerBanner } from "@/shared/components/explainer-banner";

import { UserRolesDialog } from "@/features/admin-users";
import { Alert, AlertDescription, AlertTitle } from "@/shared/components/ui/alert";

import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";

import { Skeleton } from "@/shared/components/ui/skeleton";
import { getUser, impersonate as impersonateUser, triggerEnrichDemo, userRoles } from "./api";
import { listEnrichmentProfiles } from "@/features/admin-ops";

import { useCan } from "@/shared/hooks/use-permissions";

export function AdminUserDetailPage({ userId }: { userId: string }) {
  const can = useCan();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [managingRoles, setManagingRoles] = useState(false);

  const user = useQuery({
    queryKey: ["users", "single", userId],
    queryFn: () => getUser(userId),
    retry: false,
  });
  const profiles = useQuery({
    queryKey: ["enrichment-profiles", "user", userId],
    queryFn: () => listEnrichmentProfiles({ mode: "offset", limit: 25, user_id: userId }),
  });
  const roles = useQuery({
    queryKey: ["users", userId, "roles"],
    queryFn: () => userRoles(userId),
  });

  const invalidateUser = () => {
    void queryClient.invalidateQueries({ queryKey: ["users", "single", userId] });
    void queryClient.invalidateQueries({ queryKey: ["users", userId, "roles"] });
    void queryClient.invalidateQueries({ queryKey: ["users", userId, "activity"] });
    void queryClient.invalidateQueries({ queryKey: ["users"] });
  };

  const enrichDemo = useMutation({
    mutationFn: ({ source }: { source: "kafka" | "nats" }) => triggerEnrichDemo(userId, source),
    onSuccess: (result) => {
      toast.success(
        `Published ${result.source} event ${result.event_id.slice(0, 18)}… — the worker applies it in a moment`,
      );
      void queryClient.invalidateQueries({ queryKey: ["users", "single", userId] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const impersonate = useMutation({
    mutationFn: () => impersonateUser(userId),
    onSuccess: () => {
      toast.success(`Now acting as ${user.data?.email ?? "this user"}`);
      void queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
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

  if (!user.data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-32 rounded-md" />
        <Skeleton className="h-44 w-full rounded-xl" />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-5">
          <Skeleton className="h-[520px] rounded-xl lg:col-span-3" />
          <Skeleton className="h-[520px] rounded-xl lg:col-span-2" />
        </div>
      </div>
    );
  }

  const u = user.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button variant="ghost" className="-ml-2" asChild>
          <Link to="/admin/users">
            <ArrowLeft /> All users
          </Link>
        </Button>
        <div className="flex items-center gap-2">
          {can("impersonation", "execute") ? (
            <Button
              variant="outline"
              onClick={() => impersonate.mutate()}
              disabled={impersonate.isPending}
            >
              {impersonate.isPending ? <Loader2 className="animate-spin" /> : <VenetianMask />}
              <span className="hidden sm:inline">Sign in as</span>
            </Button>
          ) : null}
          <Button onClick={() => setEditing(true)}>
            <Pencil /> Edit
          </Button>
        </div>
      </div>

      <UserHeroCard
        user={u}
        roleRules={roles.data ?? []}
        onManageRoles={() => setManagingRoles(true)}
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-5">
        {can("audit", "read") ? (
          <Card className="lg:col-span-3">
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <CalendarClock className="size-4 text-muted-foreground" /> History &amp; changes
              </CardTitle>
              <CardDescription>
                Every recorded event for this account — profile changes, role grants, sign-ins,
                impersonation — plus everything this user did themselves. Newest first.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AuditTimeline userId={u.id} className="h-[460px]" />
            </CardContent>
          </Card>
        ) : null}

        <RolesPermissionsCard
          userId={u.id}
          className={can("audit", "read") ? "lg:col-span-2" : "lg:col-span-5"}
          onManage={() => setManagingRoles(true)}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
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
          ) : (profiles.data?.data?.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">No enrichment profiles for this user.</p>
          ) : (
            profiles.data?.data?.map((profile, index) => (
              <div key={index} className="rounded-md border p-2">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span className="font-mono">{profile.source}</span>
                  <span>{format(new Date(profile.created_at), "MMM d, HH:mm")}</span>
                </div>
                <pre className="mt-1 max-h-40 overflow-x-auto text-xs">{profile.payload}</pre>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <EditUserDialog user={u} open={editing} onOpenChange={setEditing} onSaved={invalidateUser} />
      <UserRolesDialog user={managingRoles ? u : null} onOpenChange={setManagingRoles} />
    </div>
  );
}

/**
 * UserHeroCard is the identity centerpiece: the user's gradient avatar over
 * a tinted cover, their roles as chips, and the immutable account facts
 * (id, timestamps, external_ref) with one-tap copy.
 */

import { RolesPermissionsCard, UserHeroCard } from "./user-detail-parts";
import { EditUserDialog, EnrichmentCard } from "./user-detail-dialogs";
