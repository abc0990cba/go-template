import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { assignUserRole, unassignUserRole, userRoles } from "./api";
import { listRoles } from "@/features/admin-rbac";
import type { PolicyRule, User } from "@/lib/types";
import { UserAvatar } from "@/components/user-avatar";

/**
 * UserRolesDialog manages the account's role assignments in the caller's
 * active scope (or the platform). Grants map to Casbin g rules. Shared by
 * the users table and the user card so role editing looks and behaves the
 * same from both entry points.
 */
export function UserRolesDialog({
  user,
  onOpenChange,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [role, setRole] = useState("");
  const roles = useQuery({ queryKey: ["roles"], queryFn: listRoles, enabled: user !== null });
  const assignments = useQuery({
    queryKey: ["users", user?.id ?? "", "roles"],
    queryFn: () => userRoles(user?.id ?? ""),
    enabled: user !== null,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["users", user?.id ?? "", "roles"] });
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const assign = useMutation({
    mutationFn: () => assignUserRole(user?.id ?? "", role, ""),
    onSuccess: () => {
      toast.success("Role assigned");
      setRole("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const unassign = useMutation({
    mutationFn: (rule: PolicyRule) =>
      unassignUserRole(rule.subject, rule.object ?? "", rule.domain),
    onSuccess: () => {
      toast.success("Role revoked");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Dialog open={user !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Roles — {user?.email}</DialogTitle>
          <DialogDescription>
            Assignments in the platform scope; tenant roles come from the active scope selector.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2 sm:flex-row">
          <select
            className="h-11 w-full rounded-md border bg-background px-3 text-sm sm:h-9"
            value={role}
            onChange={(event) => setRole(event.target.value)}
          >
            <option value="">Choose a role…</option>
            {(roles.data ?? []).map((r) => (
              <option key={r.key} value={r.key}>
                {r.name} ({r.key})
              </option>
            ))}
          </select>
          <Button onClick={() => assign.mutate()} disabled={!role || assign.isPending}>
            Assign
          </Button>
        </div>
        <div className="space-y-1">
          {(assignments.data ?? []).map((rule) => (
            <div
              key={`${rule.subject}|${rule.object}|${rule.domain}`}
              className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
            >
              <code className="text-xs">{rule.object}</code>
              <span className="text-muted-foreground">
                @ {rule.domain === "*" ? "platform" : rule.domain.slice(0, 8)}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="ml-auto text-destructive"
                onClick={() => unassign.mutate(rule)}
                aria-label={`Revoke ${rule.object}`}
              >
                <Trash2 />
              </Button>
            </div>
          ))}
          {!assignments.isPending && (assignments.data ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">No roles assigned.</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

/** Small helper used by cards that show a user's role badges. */
export function UserRoleChip({
  user,
  roleKey,
  roleName,
  domain,
}: {
  user?: User | null;
  roleKey: string;
  roleName?: string;
  domain?: string;
}) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border bg-background px-2 py-0.5 text-xs font-medium">
      {user ? (
        <UserAvatar
          name={user.name}
          email={user.email}
          className="size-4"
          fallbackClassName="text-[8px]"
        />
      ) : null}
      {roleName || roleKey}
      {domain && domain !== "*" ? (
        <span className="text-[10px] text-muted-foreground">@{domain.slice(0, 8)}</span>
      ) : null}
    </span>
  );
}
