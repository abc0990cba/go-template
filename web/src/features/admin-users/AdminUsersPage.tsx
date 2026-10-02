import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { useTableView } from "@/shared/hooks/use-paged-list";
import { useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { Link } from "@tanstack/react-router";
import {
  ArrowUpRight,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  Trash2,
  VenetianMask,
} from "lucide-react";
import { DataTable } from "@/shared/components/data-table/data-table";
import { IconActionButton } from "@/shared/components/icon-action-button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/components/ui/tooltip";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { UserAvatar } from "@/shared/components/user-avatar";
import { CreateDialog, EditDialog, DeleteDialog } from "./users-dialogs";
import { UserRolesDialog } from "@/features/admin-users";

import { Button } from "@/shared/components/ui/button";

import { Input } from "@/shared/components/ui/input";
import { impersonate, listUsers } from "./api";
import type { ListSearch } from "@/shared/lib/search";
import type { User } from "@/shared/types/core";

export function AdminUsersPage({ search }: { search: ListSearch }) {
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

  const table = useTableView<User>("users", listUsers, params, search.view);

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
      cell: ({ row }) => (
        <div className="flex min-w-0 items-center gap-2.5">
          <UserAvatar
            name={row.original.name}
            email={row.original.email}
            className="size-9 shrink-0 md:size-8"
          />
          {/* Floor width: on phones the panel pans horizontally (actions are
              pinned) instead of squeezing the name into a sliver. */}
          <div className="min-w-36 flex-1 sm:min-w-0">
            <span className="block max-w-40 truncate font-medium sm:max-w-56">
              {row.original.name || "—"}
            </span>
            <span className="block max-w-40 truncate font-mono text-xs text-muted-foreground md:hidden">
              {row.original.email}
            </span>
          </div>
        </div>
      ),
    },
    {
      accessorKey: "email",
      header: "Email",
      meta: { cellClass: "hidden md:table-cell", headerClass: "hidden md:table-cell" },
    },
    {
      accessorKey: "created_at",
      header: "Created",
      meta: { cellClass: "hidden lg:table-cell", headerClass: "hidden lg:table-cell" },
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
      // Pinned to the panel edge: when the table pans horizontally on a
      // phone, row actions stay one thumb-tap away.
      meta: {
        headerClass: "sticky right-0 bg-card",
        cellClass: "sticky right-0 bg-card border-l border-border/60",
      },
      cell: ({ row }) => (
        <div className="flex items-center justify-end gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Link
                to="/admin/users/$userId"
                params={{ userId: row.original.id }}
                aria-label={`Open ${row.original.email}`}
                className="inline-flex size-7 items-center justify-center rounded-md hover:bg-accent"
              >
                <ArrowUpRight className="size-4" />
              </Link>
            </TooltipTrigger>
            <TooltipContent side="top">
              <p>Open details: profile, enrichment cards, fetched profiles</p>
            </TooltipContent>
          </Tooltip>
          <IconActionButton
            icon={ShieldCheck}
            tooltip="Manage roles: assign or revoke this user's roles"
            label={`Roles of ${row.original.email}`}
            onClick={() => setRolesOf(row.original)}
          />
          <IconActionButton
            icon={VenetianMask}
            tooltip="Sign in as this user (impersonation) — you act with their permissions until you stop"
            label={`Sign in as ${row.original.email}`}
            onClick={() => {
              void impersonate(row.original.id).then(
                () => {
                  void queryClient.invalidateQueries();
                },
                (e: Error) => toast.error(e.message),
              );
            }}
          />
          <IconActionButton
            icon={Pencil}
            tooltip="Edit name, email, and external_ref (the id the Kafka/NATS streams use)"
            label={`Edit ${row.original.email}`}
            onClick={() => setEditing(row.original)}
          />
          <IconActionButton
            icon={Trash2}
            tooltip="Delete this user — removes credentials, identities, and sessions; no undo"
            label={`Delete ${row.original.email}`}
            destructive
            onClick={() => setDeleting(row.original)}
          />
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
        onRowClick={(user) =>
          void navigate({ to: "/admin/users/$userId", params: { userId: user.id } })
        }
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
