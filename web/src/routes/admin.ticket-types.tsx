import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { ArrowRight, Plus, RefreshCw, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table/data-table";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import type { TicketType } from "@/lib/types";
import type { ColumnDef } from "@tanstack/react-table";

export const Route = createFileRoute("/admin/ticket-types")({
  staticData: { title: "Ticket Types" },
  component: TicketTypesPage,
});

interface StepDraft {
  name: string;
  approver_role: string;
}

/**
 * Ticket types define the approval chains behind tickets: an ordered list
 * of steps, each naming the Casbin role that signs it off. Editing a chain
 * only affects tickets submitted afterwards — in-flight tickets keep the
 * steps they were submitted with.
 */
function TicketTypesPage() {
  const queryClient = useQueryClient();
  const types = useQuery({ queryKey: ["ticket-types"], queryFn: () => api.ticketTypes() });
  const [editing, setEditing] = useState<TicketType | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const update = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      api.updateTicketType(id, { active }),
    onSuccess: () => {
      toast.success("Ticket type updated");
      void queryClient.invalidateQueries({ queryKey: ["ticket-types"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteTicketType(id),
    onSuccess: () => {
      toast.success("Ticket type deleted");
      void queryClient.invalidateQueries({ queryKey: ["ticket-types"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const columns: ColumnDef<TicketType>[] = [
    { accessorKey: "key", header: "Key", cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.key}</span>
    ) },
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
          onCheckedChange={(checked) =>
            update.mutate({ id: row.original.id, active: checked })
          }
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
          <Button size="sm" variant="outline" className="h-8 md:h-7" onClick={() => setEditing(row.original)}>
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
          <Button variant="outline" size="sm" onClick={() => void queryClient.invalidateQueries({ queryKey: ["ticket-types"] })}>
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

function TypeDialog({
  open,
  onOpenChange,
  existing,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existing?: TicketType;
}) {
  const queryClient = useQueryClient();
  const [key, setKey] = useState(existing?.key ?? "");
  const [name, setName] = useState(existing?.name ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [steps, setSteps] = useState<StepDraft[]>(
    existing?.steps.map((s) => ({ name: s.name, approver_role: s.approver_role })) ?? [
      { name: "Manager review", approver_role: "manager" },
    ],
  );

  const save = useMutation({
    mutationFn: () => {
      if (existing) {
        return api.updateTicketType(existing.id, {
          name: name || undefined,
          description,
          steps: steps.map((s) => ({ name: s.name, approver_role: s.approver_role })),
        });
      }
      return api.createTicketType(key, name, description, steps);
    },
    onSuccess: () => {
      toast.success(existing ? "Ticket type updated" : "Ticket type created");
      void queryClient.invalidateQueries({ queryKey: ["ticket-types"] });
      onOpenChange(false);
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{existing ? `Edit ${existing.name}` : "New ticket type"}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          {!existing && (
            <div className="grid gap-1.5">
              <Label htmlFor="type-key">Key</Label>
              <Input id="type-key" value={key} onChange={(e) => setKey(e.target.value)} placeholder="expense" />
            </div>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="type-name">Name</Label>
            <Input id="type-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="type-description">Description</Label>
            <Input id="type-description" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label>Approval steps (in order)</Label>
            {steps.map((step, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input
                  value={step.name}
                  onChange={(e) =>
                    setSteps(steps.map((s, i) => (i === index ? { ...s, name: e.target.value } : s)))
                  }
                  placeholder={`Step ${index + 1} name`}
                />
                <Input
                  value={step.approver_role}
                  onChange={(e) =>
                    setSteps(steps.map((s, i) => (i === index ? { ...s, approver_role: e.target.value } : s)))
                  }
                  placeholder="role"
                  className="w-40"
                />
                <Button
                  size="sm"
                  variant="outline"
                  disabled={steps.length === 1}
                  onClick={() => setSteps(steps.filter((_, i) => i !== index))}
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
            <Button
              size="sm"
              variant="outline"
              className="justify-self-start"
              onClick={() => setSteps([...steps, { name: "", approver_role: "approver" }])}
            >
              <Plus /> Add step
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!name.trim() || steps.some((s) => !s.name.trim() || !s.approver_role.trim()) || save.isPending}
            onClick={() => save.mutate()}
          >
            {existing ? "Save changes" : "Create type"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
