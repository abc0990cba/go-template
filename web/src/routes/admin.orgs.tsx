import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { format } from "date-fns";
import { Loader2, Plus, Trash2, UserPlus } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { UserAvatar } from "@/components/user-avatar";
import { api } from "@/lib/api";
import { ensurePermission } from "@/lib/auth";
import type { Organization } from "@/lib/types";

export const Route = createFileRoute("/admin/orgs")({
  staticData: { title: "Organizations" },
  beforeLoad: ({ location }) => ensurePermission("orgs", "read", location),
  component: OrgsPage,
});

function OrgsPage() {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [managing, setManaging] = useState<Organization | null>(null);
  const [deleting, setDeleting] = useState<Organization | null>(null);
  const orgs = useQuery({ queryKey: ["orgs"], queryFn: () => api.listOrgs({ limit: 100 }) });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["orgs"] });
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const removeOrg = useMutation({
    mutationFn: (id: string) => api.deleteOrg(id),
    onSuccess: () => {
      toast.success("Organization deleted");
      setDeleting(null);
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Organizations</h1>
        <p className="text-sm text-muted-foreground">
          Tenants group users into Casbin domains with their own grants.
        </p>
      </div>
      <ExplainerBanner
        title="Organizations — multi-tenancy via Casbin domains"
        howTo={[
          "Create a tenant, add members, then assign roles inside the tenant on the Users page.",
          "Switch the active scope with the header selector; every list reflects it.",
          "Removing a member also revokes their tenant roles.",
        ]}
      >
        A tenant is a domain in the policy model: <code>g, user, role, domain</code> assignments and{" "}
        <code>p, role, domain, object, action</code> grants only apply inside it, so the same person
        can be an admin of one tenant and a viewer of another.
      </ExplainerBanner>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {orgs.isPending && <Loader2 className="size-5 animate-spin" />}
        {(orgs.data?.data ?? []).map((org) => (
          <OrgCard key={org.id} org={org} onManage={() => setManaging(org)} onDelete={() => setDeleting(org)} />
        ))}
      </div>

      <div className="flex justify-start">
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus /> New organization
        </Button>
      </div>

      <CreateOrgDialog open={creating} onOpenChange={setCreating} onCreated={invalidate} />
      {managing && (
        <MembersDialog org={managing} onOpenChange={(open) => !open && setManaging(null)} />
      )}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleting?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Members lose their tenant grants immediately. There is no undo.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                if (deleting) removeOrg.mutate(deleting.id);
              }}
            >
              {removeOrg.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/**
 * OrgCard shows the tenant with its seat roster: avatar faces for the first
 * members, a "+N" counter for the rest, and a seats line. The roster comes
 * from the same members endpoint the dialog manages.
 */
function OrgCard({
  org,
  onManage,
  onDelete,
}: {
  org: Organization;
  onManage: () => void;
  onDelete: () => void;
}) {
  const members = useQuery({
    queryKey: ["orgs", org.id, "members"],
    queryFn: () => api.orgMembers(org.id),
  });
  const roster = (members.data ?? []).slice(0, 4);
  const extra = Math.max((members.data?.length ?? 0) - roster.length, 0);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center justify-between text-base">
          {org.name}
          <code className="text-xs font-normal text-muted-foreground">{org.slug}</code>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-3">
          <div className="flex items-center">
            {members.isPending ? (
              <Skeleton className="h-7 w-24 rounded-full" />
            ) : (
              roster.map((member) => (
                <UserAvatar
                  key={member.user_id}
                  name={member.name}
                  email={member.email}
                  className="size-7 ring-2 ring-card [&:not(:first-child)]:-ml-2"
                  fallbackClassName="text-[10px]"
                />
              ))
            )}
            {extra > 0 ? (
              <span className="-ml-2 flex size-7 items-center justify-center rounded-full bg-muted text-[10px] font-semibold text-muted-foreground ring-2 ring-card">
                +{extra}
              </span>
            ) : null}
            {roster.length === 0 && !members.isPending ? (
              <span className="text-xs text-muted-foreground">No members yet</span>
            ) : null}
          </div>
          {!members.isPending && (members.data?.length ?? 0) > 0 ? (
            <Badge variant="secondary" className="rounded-full">
              {(members.data?.length ?? 0)} seat{(members.data?.length ?? 0) === 1 ? "" : "s"}
            </Badge>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">
          Created {format(new Date(org.created_at), "MMM d, yyyy")}
        </p>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={onManage}>
            <UserPlus /> Seats
          </Button>
          <Button size="sm" variant="outline" className="text-destructive" onClick={onDelete}>
            <Trash2 />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function CreateOrgDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const create = useMutation({
    mutationFn: () => api.createOrg(name, slug),
    onSuccess: () => {
      toast.success("Organization created");
      setName("");
      setSlug("");
      onOpenChange(false);
      onCreated();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New organization</DialogTitle>
          <DialogDescription>
            The slug becomes the tenant id shown in role matrices; leave it empty to derive it from
            the name.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input
            placeholder="Slug (optional)"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button onClick={() => create.mutate()} disabled={create.isPending || !name}>
            {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MembersDialog({
  org,
  onOpenChange,
}: {
  org: Organization;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [userId, setUserId] = useState("");
  const members = useQuery({
    queryKey: ["orgs", org.id, "members"],
    queryFn: () => api.orgMembers(org.id),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["orgs", org.id, "members"] });
  };

  const add = useMutation({
    mutationFn: () => api.addOrgMember(org.id, userId.trim()),
    onSuccess: () => {
      toast.success("Member added");
      setUserId("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const remove = useMutation({
    mutationFn: (memberId: string) => api.removeOrgMember(org.id, memberId),
    onSuccess: () => {
      toast.success("Member removed");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Seats — {org.name}</DialogTitle>
          <DialogDescription>
            {(members.data?.length ?? 0)} member{(members.data?.length ?? 0) === 1 ? "" : "s"}{" "}
            occupy this tenant. Paste a user id to add them; assign their tenant role afterwards on
            the Users page.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input placeholder="user id" value={userId} onChange={(e) => setUserId(e.target.value)} />
          <Button onClick={() => add.mutate()} disabled={!userId.trim() || add.isPending}>
            <UserPlus /> Add
          </Button>
        </div>
        <div className="space-y-1">
          {(members.data ?? []).map((member) => (
            <div
              key={member.user_id}
              className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
            >
              <UserAvatar
                name={member.name}
                email={member.email}
                className="size-7 shrink-0"
                fallbackClassName="text-[10px]"
              />
              <div className="min-w-0 flex-1">
                <span className="block truncate font-medium">{member.name || "—"}</span>
                <span className="block truncate text-xs text-muted-foreground">{member.email}</span>
              </div>
              <Badge variant="outline" className="hidden font-mono text-[10px] sm:inline-flex">
                {member.user_id.slice(0, 8)}
              </Badge>
              <Button
                variant="ghost"
                size="icon"
                className="text-destructive"
                onClick={() => remove.mutate(member.user_id)}
                aria-label={`Remove ${member.email}`}
              >
                <Trash2 />
              </Button>
            </div>
          ))}
          {!members.isPending && (members.data ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">No members yet.</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
