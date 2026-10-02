import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { format } from "date-fns";
import {
  ArrowLeft,
  BadgeCheck,
  CalendarClock,
  Check,
  Copy,
  Fingerprint,
  KeySquare,
  Link2,
  Loader2,
  Pencil,
  RefreshCw,
  Send,
  ShieldCheck,
  VenetianMask,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { AuditTimeline } from "@/features/admin-insights";
import { ExplainerBanner } from "@/components/explainer-banner";
import { UserAvatar } from "@/components/user-avatar";
import { UserRolesDialog } from "@/features/admin-users";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  getUser,
  impersonate as impersonateUser,
  triggerEnrichDemo,
  updateUser,
  userRoles,
} from "./api";
import { listEnrichmentProfiles } from "@/features/admin-ops";
import { listRoles, rolePermissions } from "@/features/admin-rbac";
import { useCan } from "@/shared/hooks/use-permissions";
import type { User } from "@/shared/types/core";

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
function UserHeroCard({
  user,
  roleRules,
  onManageRoles,
}: {
  user: User;
  roleRules: { object?: string; domain: string }[];
  onManageRoles: () => void;
}) {
  const [copied, setCopied] = useState("");

  const copy = (value: string, label: string) => {
    void navigator.clipboard?.writeText(value).then(
      () => {
        setCopied(label);
        toast.success(`${label} copied`);
        window.setTimeout(() => setCopied(""), 1500);
      },
      () => toast.error("Clipboard unavailable"),
    );
  };

  return (
    <Card className="gap-0 overflow-hidden py-0">
      {/* Cover band tinted from the same palette family as the avatar. */}
      <div className="relative h-20 bg-linear-to-r from-sky-500/15 via-violet-500/15 to-fuchsia-500/15 sm:h-24">
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top_right,theme(colors.sky.500/10),transparent_55%)]" />
      </div>
      <CardContent className="space-y-4 pb-5">
        <div className="-mt-10 flex flex-col gap-4 sm:-mt-12 sm:flex-row sm:items-end">
          <UserAvatar
            name={user.name}
            email={user.email}
            className="size-20 border-4 border-card shadow-md sm:size-24"
            fallbackClassName="text-2xl sm:text-3xl"
          />
          <div className="min-w-0 flex-1 pb-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold tracking-tight sm:text-2xl">
                {user.name || "Unnamed user"}
              </h1>
              <Badge variant="secondary" className="gap-1 rounded-full">
                <BadgeCheck className="size-3" /> active
              </Badge>
            </div>
            <p className="truncate font-mono text-sm text-muted-foreground">{user.email}</p>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {roleRules.length === 0 ? (
                <span className="text-xs text-muted-foreground">No roles assigned</span>
              ) : (
                roleRules.map((rule) => (
                  <Badge
                    key={`${rule.object}-${rule.domain}`}
                    variant="outline"
                    className="gap-1 rounded-full bg-primary/5"
                  >
                    <ShieldCheck className="size-3 text-violet-500" />
                    {rule.object}
                    {rule.domain !== "*" ? (
                      <span className="text-[10px] text-muted-foreground">
                        @{rule.domain.slice(0, 8)}
                      </span>
                    ) : null}
                  </Badge>
                ))
              )}
              <Button
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2 text-xs text-muted-foreground"
                onClick={onManageRoles}
              >
                <ShieldCheck className="size-3" /> Manage
              </Button>
            </div>
          </div>
        </div>

        <div className="grid gap-x-6 gap-y-3 border-t pt-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Fact label="User id" mono>
            <span className="truncate font-mono text-xs">{user.id}</span>
            <button
              type="button"
              aria-label="Copy user id"
              className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
              onClick={() => copy(user.id, "User id")}
            >
              {copied === "User id" ? (
                <Check className="size-3.5" />
              ) : (
                <Copy className="size-3.5" />
              )}
            </button>
          </Fact>
          <Fact label="Created">{format(new Date(user.created_at), "MMM d, yyyy HH:mm")}</Fact>
          <Fact label="Updated">{format(new Date(user.updated_at), "MMM d, yyyy HH:mm")}</Fact>
          <Fact label="External ref" mono>
            {user.external_ref ? (
              <>
                <span className="truncate font-mono text-xs">{user.external_ref}</span>
                <button
                  type="button"
                  aria-label="Copy external ref"
                  className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => copy(user.external_ref ?? "", "External ref")}
                >
                  {copied === "External ref" ? (
                    <Check className="size-3.5" />
                  ) : (
                    <Copy className="size-3.5" />
                  )}
                </button>
              </>
            ) : (
              <span className="text-xs text-muted-foreground">not set</span>
            )}
          </Fact>
        </div>
      </CardContent>
    </Card>
  );
}

function Fact({
  label,
  mono = false,
  children,
}: {
  label: string;
  mono?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <p className="flex items-center gap-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
        {mono ? <Fingerprint className="size-3" /> : null}
        {label}
      </p>
      <div className={`mt-0.5 flex items-center gap-1 ${mono ? "min-w-0" : ""}`}>{children}</div>
    </div>
  );
}

/**
 * RolesPermissionsCard resolves each assigned role to its permission matrix:
 * role headers with domain badges, every object:action grant as a chip.
 */
function RolesPermissionsCard({
  userId,
  className,
  onManage,
}: {
  userId: string;
  className?: string;
  onManage: () => void;
}) {
  const assignments = useQuery({
    queryKey: ["users", userId, "roles"],
    queryFn: () => userRoles(userId),
  });
  const catalog = useQuery({ queryKey: ["roles"], queryFn: listRoles });

  const roleKeys = [...new Set((assignments.data ?? []).map((rule) => rule.object ?? ""))].filter(
    Boolean,
  );

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeySquare className="size-4 text-muted-foreground" /> Roles &amp; permissions
            </CardTitle>
            <CardDescription>
              What this account may do: every grant behind the assigned roles.
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={onManage}>
            <ShieldCheck /> Manage
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {assignments.isPending ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-3/4" />
          </div>
        ) : roleKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No roles assigned — this account has no explicit grants in this scope.
          </p>
        ) : (
          roleKeys.map((key) => <RoleBlock key={key} roleKey={key} catalog={catalog.data ?? []} />)
        )}
      </CardContent>
    </Card>
  );
}

function RoleBlock({
  roleKey,
  catalog,
}: {
  roleKey: string;
  catalog: { key: string; name: string; description: string }[];
}) {
  const permissions = useQuery({
    queryKey: ["roles", roleKey, "permissions"],
    queryFn: () => rolePermissions(roleKey),
  });
  const meta = catalog.find((role) => role.key === roleKey);

  return (
    <div className="rounded-lg border p-3">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <p className="text-sm font-semibold">{meta?.name ?? roleKey}</p>
        <code className="text-[10px] text-muted-foreground">{roleKey}</code>
      </div>
      {meta?.description ? (
        <p className="mt-0.5 text-xs text-muted-foreground">{meta.description}</p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-1">
        {permissions.isPending ? (
          <Skeleton className="h-5 w-40" />
        ) : (permissions.data?.length ?? 0) === 0 ? (
          <span className="text-xs text-muted-foreground">
            No permissions granted to this role.
          </span>
        ) : (
          permissions.data?.map((rule) => (
            <span
              key={`${rule.domain}-${rule.object}-${rule.action}`}
              className="rounded-md border bg-muted/50 px-1.5 py-0.5 font-mono text-[11px]"
              title={rule.domain === "*" ? "platform scope" : `org ${rule.domain}`}
            >
              {rule.object}:{rule.action}
            </span>
          ))
        )}
      </div>
    </div>
  );
}

/** Edit dialog for the card: name, email, and the streams' external_ref. */
function EditUserDialog({
  user,
  open,
  onOpenChange,
  onSaved,
}: {
  user: User;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [email, setEmail] = useState(user.email);
  const [name, setName] = useState(user.name);
  const [externalRef, setExternalRef] = useState(user.external_ref ?? "");

  const update = useMutation({
    mutationFn: () =>
      updateUser(user.id, email.trim(), name.trim(), externalRef.trim() || undefined),
    onSuccess: () => {
      toast.success("User updated");
      onOpenChange(false);
      onSaved();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit user</DialogTitle>
          <DialogDescription>
            Updates the account via PUT /api/users/{user.id.slice(0, 8)}… — the change is audited.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-email">Email</Label>
            <Input
              id="edit-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-ref" className="flex items-center gap-1">
              <Link2 className="size-3.5" /> External ref
            </Label>
            <Input
              id="edit-ref"
              value={externalRef}
              placeholder="correlation id for Kafka/NATS streams"
              onChange={(event) => setExternalRef(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => update.mutate()} disabled={update.isPending || !email.trim()}>
            {update.isPending ? <Loader2 className="animate-spin" /> : <Check />} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
        <CardTitle className="flex items-center gap-2 text-base">
          <RefreshCw className="size-4 text-muted-foreground" /> {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="min-w-0 text-xs text-muted-foreground">
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
          <Button
            size="sm"
            variant="outline"
            className="shrink-0"
            disabled={testing || !externalRef}
            onClick={onTest}
          >
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
