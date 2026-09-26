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
import { api } from "@/lib/api";
import { ensurePermission } from "@/lib/auth";
import type { Organization } from "@/lib/types";

export const Route = createFileRoute("/_auth/orgs")({
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

      <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
        {orgs.isPending && <Loader2 className="size-5 animate-spin" />}
        {(orgs.data?.data ?? []).map((org) => (
          <Card key={org.id}>
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center justify-between text-base">
                {org.name}
                <code className="text-xs font-normal text-muted-foreground">{org.slug}</code>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <p className="text-xs text-muted-foreground">
                Created {format(new Date(org.created_at), "MMM d, yyyy")}
              </p>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={() => setManaging(org)}>
                  <UserPlus /> Members
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  onClick={() => setDeleting(org)}
                >
                  <Trash2 />
                </Button>
              </div>
            </CardContent>
          </Card>
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
          <DialogTitle>Members — {org.name}</DialogTitle>
          <DialogDescription>
            Paste a user id to add them; assign their tenant role afterwards on the Users page.
          </DialogDescription>
        </DialogHeader>
        <div className="flex gap-2">
          <Input placeholder="user id" value={userId} onChange={(e) => setUserId(e.target.value)} />
          <Button size="sm" onClick={() => add.mutate()} disabled={!userId.trim() || add.isPending}>
            <UserPlus /> Add
          </Button>
        </div>
        <div className="space-y-1">
          {(members.data ?? []).map((member) => (
            <div
              key={member.user_id}
              className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
            >
              <Badge variant="outline" className="font-mono text-[10px]">
                {member.user_id.slice(0, 8)}
              </Badge>
              <span className="font-medium">{member.name || "—"}</span>
              <span className="text-muted-foreground">{member.email}</span>
              <Button
                variant="ghost"
                size="icon"
                className="ml-auto size-6 text-destructive"
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
