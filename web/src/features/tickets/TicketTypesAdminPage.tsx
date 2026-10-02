import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { ArrowRight, Plus, RefreshCw, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import type { ColumnDef } from "@tanstack/react-table";
import { deleteTicketType, ticketTypes, updateTicketType } from "./api";
import { TypeDialog } from "./admin/TypeDialog";
import type { TicketType } from "./types";

/**
 * Ticket types define the approval chains behind tickets: an ordered list
 * of steps, each naming the Casbin role that signs it off. Editing a chain
 * only affects tickets submitted afterwards — in-flight tickets keep the
 * steps they were submitted with.
 */
export function TicketTypesPage() {
  const queryClient = useQueryClient();
  const types = useQuery({ queryKey: ["ticket-types"], queryFn: () => ticketTypes() });
  const [editing, setEditing] = useState<TicketType | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const update = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      updateTicketType(id, { active }),
    onSuccess: () => {
      toast.success("Ticket type updated");
      void queryClient.invalidateQueries({ queryKey: ["ticket-types"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteTicketType(id),
    onSuccess: () => {
      toast.success("Ticket type deleted");
      void queryClient.invalidateQueries({ queryKey: ["ticket-types"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const columns: ColumnDef<TicketType>[] = [
    {
      accessorKey: "key",
      header: "Key",
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.key}</span>,
    },
    { accessorKey: "name", header: "Name" },
    {
      id: "chain",
      header: "Approval chain",
      enableSorting: false,
      cell: ({ row }) => (
        <div className="flex flex-wrap items-center gap-1">
          {row.original.steps.map((step, index) => (
            <span key={index} className="flex items-center gap-1">
              {index > 0 && <ArrowRight className="size-3 text-muted-foreground" />}
              <Badge variant="outline">
                {step.name} · {step.approver_role}
              </Badge>
            </span>
          ))}
        </div>
      ),
    },
    {
      accessorKey: "active",
      header: "Active",
      cell: ({ row }) => (
        <Switch
          checked={row.original.active}
          onCheckedChange={(checked) => update.mutate({ id: row.original.id, active: checked })}
          aria-label="Toggle ticket type"
        />
      ),
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {format(new Date(row.original.created_at), "MMM d, yyyy")}
        </span>
      ),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <div className="flex justify-end gap-1">
          <Button
            size="sm"
            variant="outline"
            className="h-8 md:h-7"
            onClick={() => setEditing(row.original)}
          >
            Edit
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="h-8 md:h-7 text-destructive"
            onClick={() => remove.mutate(row.original.id)}
          >
            <Trash2 /> Delete
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Ticket Types</h1>
          <p className="text-sm text-muted-foreground">
            The approval chains behind every ticket: steps run in order, each signed off by the
            named role.
          </p>
        </div>
        <div className="ml-auto flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void queryClient.invalidateQueries({ queryKey: ["ticket-types"] })}
          >
            <RefreshCw /> Refresh
          </Button>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus /> New type
          </Button>
        </div>
      </div>

      <ExplainerBanner
        title="Chains are data, not code"
        howTo={[
          "A step names a Casbin role — anyone holding that role in the ticket's organization can decide it.",
          "Retire a type with the Active switch: existing drafts can no longer be submitted with it.",
          "In-flight tickets keep the chain they were submitted with; edits apply to new submissions only.",
        ]}
      >
        The requester files a ticket of one of these types at /tickets; submitting materializes one
        pending approval row per step. Delete a type only when its history no longer matters —
        existing tickets keep a type_key snapshot.
      </ExplainerBanner>

      <DataTable
        columns={columns}
        data={types.data}
        isPending={types.isPending}
        isPlaceholderData={false}
        emptyState="No ticket types yet — create the first one."
      />

      <TypeDialog open={createOpen} onOpenChange={setCreateOpen} />
      <TypeDialog
        open={editing !== null}
        onOpenChange={(open) => !open && setEditing(null)}
        existing={editing ?? undefined}
      />
    </div>
  );
}
