import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTableView } from "@/hooks/use-paged-list";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { Link } from "@tanstack/react-router";
import {
  ArrowUpRight,
  Loader2,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  Trash2,
  VenetianMask,
} from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
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
import { Button } from "@/components/ui/button";
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
import { listSearchSchema, type ListSearch } from "@/lib/search";
import type { PolicyRule, User } from "@/lib/types";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";

export const Route = createFileRoute("/admin/users")({
  validateSearch: listSearchSchema,
  staticData: { title: "Users" },
  component: UsersPage,
});

function UsersPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState<User | null>(null);
  const [editing, setEditing] = useState<User | null>(null);
  const [rolesOf, setRolesOf] = useState<User | null>(null);
  const [creating, setCreating] = useState(false);

  const limit = 10;
  const params = {
    mode: search.mode,
    limit,
    offset: search.mode === "offset" ? (search.page - 1) * limit : 0,
    after: search.after,
    q: search.q,
  };

  const table = useTableView<User>("users", params, search.view);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["users"] });
    void queryClient.invalidateQueries({ queryKey: ["analytics"] });
  };

  const setSearch = (patch: Partial<ListSearch>) => {
    void navigate({ to: "/admin/users", search: { ...search, ...patch } });
  };

  const columns: ColumnDef<User>[] = [
    {
      accessorKey: "name",
      header: "Name",
      cell: ({ row }) => <span className="font-medium">{row.original.name || "—"}</span>,
    },
    {
      accessorKey: "email",
      header: "Email",
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, yyyy HH:mm")}
        </span>
      ),
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      cell: ({ row }) => (
        <div className="flex items-center justify-end gap-1">
          <Button variant="ghost" size="icon" className="size-7" asChild>
            <Link
              to="/admin/users/$userId"
              params={{ userId: row.original.id }}
              aria-label={`Open ${row.original.email}`}
            >
              <ArrowUpRight />
            </Link>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => setRolesOf(row.original)}
            aria-label={`Roles of ${row.original.email}`}
          >
            <ShieldCheck />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => {
              void api.impersonate(row.original.id).then(
                () => {
                  void queryClient.invalidateQueries();
                },
                (e: Error) => toast.error(e.message),
              );
            }}
            aria-label={`Sign in as ${row.original.email}`}
          >
            <VenetianMask />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => setEditing(row.original)}
            aria-label={`Edit ${row.original.email}`}
          >
            <Pencil />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7 text-destructive"
            onClick={() => setDeleting(row.original)}
            aria-label={`Delete ${row.original.email}`}
          >
            <Trash2 />
          </Button>
        </div>
      ),
    },
  ];

  const hasMore = table.hasMore;

  return (
    <div className="space-y-4">
      <PageIntro />
      <ExplainerBanner
        title="Users — one table, three pagination strategies"
        howTo={[
          "Switch strategies and views with the segmented controls under the table; page or cursor state lives in the URL, so back/forward work.",
          "Create/edit dialogs and delete confirmations call POST/PUT/DELETE /api/users.",
          "Search is server-side: type at least 3 characters and the query re-runs with q=.",
        ]}
      >
        This page exists to make pagination tangible. Offset mode is the familiar LIMIT/OFFSET;
        cursor mode resumes from an opaque continuation key; keyset mode filters directly on the{" "}
        <code>(created_at DESC, id ASC)</code> index, which is why it stays fast on the 100k-user
        performance dataset. The view switcher swaps the same strategies into an infinite-scroll
        feed. The backend implements all three for every collection — this table is where you can
        feel the difference.
      </ExplainerBanner>

      <DataTable
        columns={columns}
        data={table.rows}
        isPending={table.isPending}
        isPlaceholderData={table.isPlaceholderData}
        emptyState={
          search.q ? `No users match “${search.q}”.` : "No users yet — create the first one."
        }
        pagination={{
          mode: search.mode,
          view: search.view,
          page: search.page,
          hasMore,
          onFirstPage: () => setSearch({ page: 1, after: undefined }),
          onPreviousPage: () => setSearch({ page: Math.max(1, search.page - 1) }),
          onNextPage: () => {
            if (search.mode === "offset") {
              setSearch({ page: search.page + 1 });
            } else {
              setSearch({ page: search.page + 1, after: table.nextCursor });
            }
          },
          onModeChange: (mode) => setSearch({ mode, page: 1, after: undefined }),
          onViewChange: (view) => setSearch({ view, page: 1, after: undefined }),
          onLoadMore: table.loadMore,
          isFetchingMore: table.isFetchingMore,
          loadedCount: table.loadedCount,
        }}
        toolbar={
          <>
            <SearchBox
              value={search.q ?? ""}
              onCommit={(q) => setSearch({ q: q || undefined, page: 1, after: undefined })}
            />
            <div className="ml-auto">
              <Button size="sm" onClick={() => setCreating(true)}>
                <Plus /> New user
              </Button>
            </div>
          </>
        }
      />

      <CreateDialog open={creating} onOpenChange={setCreating} onCreated={invalidate} />
      <EditDialog
        user={editing}
        onOpenChange={(open) => !open && setEditing(null)}
        onSaved={invalidate}
      />
      <UserRolesDialog user={rolesOf} onOpenChange={(open) => !open && setRolesOf(null)} />
      <DeleteDialog
        user={deleting}
        onOpenChange={(open) => !open && setDeleting(null)}
        onDeleted={invalidate}
      />
    </div>
  );
}

function PageIntro() {
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight">Users</h1>
      <p className="text-sm text-muted-foreground">
        Accounts in the system, with live search and full CRUD.
      </p>
    </div>
  );
}

function SearchBox({ value, onCommit }: { value: string; onCommit: (q: string) => void }) {
  const [text, setText] = useState(value);
  return (
    <form
      className="relative w-full max-w-xs"
      onSubmit={(event) => {
        event.preventDefault();
        onCommit(text.trim());
      }}
    >
      <Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
      <Input
        className="pl-8"
        placeholder="Search name or email, then Enter"
        value={text}
        onChange={(event) => setText(event.target.value)}
        onBlur={() => onCommit(text.trim())}
      />
    </form>
  );
}

const userFormSchema = z.object({
  name: z.string(),
  email: z.string().email("Enter a valid email"),
});

type UserForm = z.infer<typeof userFormSchema>;

function UserFormFields({ form }: { form: ReturnType<typeof useForm<UserForm>> }) {
  return (
    <>
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Name</FormLabel>
            <FormControl>
              <Input placeholder="Ada Lovelace" {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Email</FormLabel>
            <FormControl>
              <Input type="email" placeholder="ada@example.com" {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

function CreateDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const form = useForm<UserForm>({
    resolver: zodResolver(userFormSchema),
    defaultValues: { name: "", email: "" },
  });
  const create = useMutation({
    mutationFn: (values: UserForm) => api.createUser(values.email, values.name),
    onSuccess: (user) => {
      toast.success(`Created ${user.email}`);
      form.reset();
      onOpenChange(false);
      onCreated();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New user</DialogTitle>
          <DialogDescription>
            Creates a passwordless account, as if provisioned by an external system.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id="create-user"
            onSubmit={form.handleSubmit((values) => create.mutate(values))}
            className="space-y-3"
          >
            <UserFormFields form={form} />
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form="create-user" disabled={create.isPending}>
            {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditDialog({
  user,
  onOpenChange,
  onSaved,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const form = useForm<UserForm>({
    resolver: zodResolver(userFormSchema),
    values: { name: user?.name ?? "", email: user?.email ?? "" },
  });
  const update = useMutation({
    mutationFn: (values: UserForm) => api.updateUser(user?.id ?? "", values.email, values.name),
    onSuccess: () => {
      toast.success("User updated");
      onOpenChange(false);
      onSaved();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={user !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit user</DialogTitle>
          <DialogDescription>
            Updates email and name via PUT /api/users/{user?.id}.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id="edit-user"
            onSubmit={form.handleSubmit((values) => update.mutate(values))}
            className="space-y-3"
          >
            <UserFormFields form={form} />
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form="edit-user" disabled={update.isPending}>
            {update.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteDialog({
  user,
  onOpenChange,
  onDeleted,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}) {
  const remove = useMutation({
    mutationFn: (id: string) => api.deleteUser(id),
    onSuccess: () => {
      toast.success("User deleted");
      onOpenChange(false);
      onDeleted();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <AlertDialog open={user !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {user?.email}?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the account together with its credentials, identities, and sessions. There
            is no undo.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            onClick={(event) => {
              event.preventDefault();
              if (user) remove.mutate(user.id);
            }}
          >
            {remove.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * UserRolesDialog manages the account's role assignments in the caller's
 * active scope (or the platform). Grants map to Casbin g rules.
 */
function UserRolesDialog({
  user,
  onOpenChange,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [role, setRole] = useState("");
  const roles = useQuery({ queryKey: ["roles"], queryFn: api.listRoles, enabled: user !== null });
  const assignments = useQuery({
    queryKey: ["users", user?.id ?? "", "roles"],
    queryFn: () => api.userRoles(user?.id ?? ""),
    enabled: user !== null,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["users", user?.id ?? "", "roles"] });
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const assign = useMutation({
    mutationFn: () => api.assignUserRole(user?.id ?? "", role, ""),
    onSuccess: () => {
      toast.success("Role assigned");
      setRole("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const unassign = useMutation({
    mutationFn: (rule: PolicyRule) =>
      api.unassignUserRole(rule.subject, rule.object ?? "", rule.domain),
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
        <div className="flex gap-2">
          <select
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
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
          <Button size="sm" onClick={() => assign.mutate()} disabled={!role || assign.isPending}>
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
                className="ml-auto size-6 text-destructive"
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
