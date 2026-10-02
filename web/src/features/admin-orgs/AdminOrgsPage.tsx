import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Loader2, Plus } from "lucide-react";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/components/ui/alert-dialog";

import { Button } from "@/shared/components/ui/button";

import { deleteOrg, listOrgs } from "./api";
import type { Organization } from "./types";
// Invitations share the org members dialog surface.

export function AdminOrgsPage() {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [managing, setManaging] = useState<Organization | null>(null);
  const [deleting, setDeleting] = useState<Organization | null>(null);
  const orgs = useQuery({ queryKey: ["orgs"], queryFn: () => listOrgs({ limit: 100 }) });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["orgs"] });
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const removeOrg = useMutation({
    mutationFn: (id: string) => deleteOrg(id),
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
          <OrgCard
            key={org.id}
            org={org}
            onManage={() => setManaging(org)}
            onDelete={() => setDeleting(org)}
          />
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

import { OrgCard, CreateOrgDialog, MembersDialog } from "./orgs-parts";
