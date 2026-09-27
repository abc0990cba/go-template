import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { DataTable } from "@/components/data-table/data-table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { useMe } from "@/hooks/use-permissions";
import { ensurePermission } from "@/lib/auth";
import type { Role, RbacResource } from "@/lib/types";

export const Route = createFileRoute("/admin/roles")({
  staticData: { title: "Roles" },
  beforeLoad: ({ location }) => ensurePermission("roles", "read", location),
  component: RolesPage,
});

function RolesPage() {
  const me = useMe();
  const [selected, setSelected] = useState<Role | null>(null);
  const [creating, setCreating] = useState(false);
  const roles = useQuery({ queryKey: ["roles"], queryFn: api.listRoles });

  const columns = [
    { accessorKey: "name", header: "Role" },
    {
      accessorKey: "key",
      header: "Key",
      cell: (c: { getValue: () => unknown }) => (
        <code className="text-xs">{String(c.getValue())}</code>
      ),
    },
    { accessorKey: "description", header: "Description" },
    {
      accessorKey: "is_system",
      header: "Origin",
      cell: (c: { getValue: () => unknown }) =>
        c.getValue() ? (
          <Badge variant="secondary">system</Badge>
        ) : (
          <Badge variant="outline">custom</Badge>
        ),
    },
    {
      id: "actions",
      header: "",
      cell: (c: { row: { original: Role } }) => (
        <div className="flex items-center justify-end gap-1">
          <Button variant="ghost" size="sm" onClick={() => setSelected(c.row.original)}>
            <ShieldCheck /> Permissions
          </Button>
          {!c.row.original.is_system && (
            <Button
              variant="ghost"
              size="icon"
              className="size-7 text-destructive"
              aria-label={`Delete ${c.row.original.key}`}
              onClick={() => {
                const remove = api.deleteRole(c.row.original.key);
                void remove.then(
                  () => {
                    toast.success("Role deleted");
                    void roles.refetch();
                  },
                  (e: Error) => toast.error(e.message),
                );
              }}
            >
              <Trash2 />
            </Button>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Roles</h1>
        <p className="text-sm text-muted-foreground">
          Signed in as {me.roles.join(", ") || "no role"} — switch the scope in the header to edit
          tenant-specific grants.
        </p>
      </div>
      <ExplainerBanner
        title="Roles — Casbin RBAC with domains"
        howTo={[
          "Pick a role and toggle resources × actions; grants are stored as p rules for the active scope.",
          "System roles (admin, manager, viewer…) are protected from deletion but their matrix is editable.",
          "Custom roles are usable immediately: assign them on the Users page or via raw policies.",
        ]}
      >
        The permission matrix writes <code>p, role, domain, object, action</code> rules into the
        policy store. The backend enforces them on every API route; the SPA hides what you cannot
        do. Wildcards: <code>*</code> matches everything.
      </ExplainerBanner>

      <DataTable
        columns={columns}
        data={roles.data}
        isPending={roles.isPending}
        isPlaceholderData={roles.isPlaceholderData}
        emptyState="No roles yet."
        toolbar={
          <div className="ml-auto">
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus /> New role
            </Button>
          </div>
        }
      />

      <CreateRoleDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => void roles.refetch()}
      />
      {selected && (
        <PermissionMatrix role={selected} onOpenChange={(open) => !open && setSelected(null)} />
      )}
    </div>
  );
}

function CreateRoleDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const create = useMutation({
    mutationFn: () => api.createRole(key.trim().toLowerCase(), name, description),
    onSuccess: () => {
      toast.success("Role created");
      setKey("");
      setName("");
      setDescription("");
      onOpenChange(false);
      onCreated();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New role</DialogTitle>
          <DialogDescription>
            The key is the Casbin role id; lowercase letters, digits, and dashes.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            placeholder="key (e.g. billing-editor)"
            value={key}
            onChange={(e) => setKey(e.target.value)}
          />
          <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input
            placeholder="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button onClick={() => create.mutate()} disabled={create.isPending || !key || !name}>
            {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PermissionMatrix({
  role,
  onOpenChange,
}: {
  role: Role;
  onOpenChange: (open: boolean) => void;
}) {
  const me = useMe();
  const queryClient = useQueryClient();
  const catalog = useQuery({ queryKey: ["rbac", "catalog"], queryFn: api.rbacCatalog });
  const current = useQuery({
    queryKey: ["roles", role.key, "permissions"],
    queryFn: () => api.rolePermissions(role.key),
  });
  const [domain, setDomain] = useState(me.active_org || "*");
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [loaded, setLoaded] = useState<string | null>(null);

  // Load the role's rules for the chosen scope into local checkbox state.
  // Only exact-scope rules are shown: platform rules apply everywhere but
  // are edited in the platform scope, not here.
  if (current.data && loaded !== domain) {
    const next = new Set<string>();
    for (const rule of current.data) {
      if (rule.ptype === "p" && rule.domain === domain) {
        next.add(`${rule.object}:${rule.action}`);
      }
    }
    setChecked(next);
    setLoaded(domain);
  }

  const save = useMutation({
    mutationFn: () =>
      api.setRolePermissions(
        role.key,
        domain,
        [...checked].map((entry) => {
          const [object, action] = entry.split(":");
          return { object: object ?? "", action: action ?? "" };
        }),
      ),
    onSuccess: () => {
      toast.success(`Permissions saved for ${role.key}`);
      void queryClient.invalidateQueries({ queryKey: ["roles"] });
      void queryClient.invalidateQueries({ queryKey: ["me"] });
      onOpenChange(false);
    },
    onError: (error) => toast.error(error.message),
  });

  const toggle = (resource: RbacResource, action: string) => {
    setChecked((previous) => {
      const next = new Set(previous);
      const entry = `${resource.key}:${action}`;
      if (next.has(entry)) next.delete(entry);
      else next.add(entry);
      return next;
    });
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            Permissions — <code className="text-sm">{role.key}</code>
          </DialogTitle>
          <DialogDescription>
            Resources × actions granted in the selected scope. Saving replaces the role&apos;s rules
            in that scope.
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Scope</span>
          <Select value={domain} onValueChange={setDomain}>
            <SelectTrigger className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="*">Platform (global)</SelectItem>
              {me.orgs.map((org) => (
                <SelectItem key={org.id} value={org.id}>
                  {org.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Card>
          <CardHeader className="py-3">
            <CardTitle className="text-sm">Matrix</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {(catalog.data ?? []).map((resource) => (
              <div
                key={resource.key}
                className="flex flex-wrap items-center gap-3 border-b pb-2 last:border-b-0"
              >
                <div className="w-40 shrink-0">
                  <code className="text-xs">{resource.key}</code>
                </div>
                {resource.actions.map((action) => {
                  const entry = `${resource.key}:${action}`;
                  return (
                    <label key={action} className="flex items-center gap-1.5 text-sm">
                      <Checkbox
                        checked={checked.has(entry)}
                        onCheckedChange={() => toggle(resource, action)}
                      />
                      {action}
                    </label>
                  );
                })}
              </div>
            ))}
          </CardContent>
        </Card>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => save.mutate()} disabled={save.isPending}>
            {save.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
