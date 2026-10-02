import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Plus, Trash2 } from "lucide-react";
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
import { createTicketType, updateTicketType } from "../api";
import type { TicketType } from "../types";

interface StepDraft {
  name: string;
  approver_role: string;
}

export function TypeDialog({
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
        return updateTicketType(existing.id, {
          name: name || undefined,
          description,
          steps: steps.map((s) => ({ name: s.name, approver_role: s.approver_role })),
        });
      }
      return createTicketType(key, name, description, steps);
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
              <Input
                id="type-key"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="expense"
              />
            </div>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="type-name">Name</Label>
            <Input id="type-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="type-description">Description</Label>
            <Input
              id="type-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>Approval steps (in order)</Label>
            {steps.map((step, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input
                  value={step.name}
                  onChange={(e) =>
                    setSteps(
                      steps.map((s, i) => (i === index ? { ...s, name: e.target.value } : s)),
                    )
                  }
                  placeholder={`Step ${index + 1} name`}
                />
                <Input
                  value={step.approver_role}
                  onChange={(e) =>
                    setSteps(
                      steps.map((s, i) =>
                        i === index ? { ...s, approver_role: e.target.value } : s,
                      ),
                    )
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
            disabled={
              !name.trim() ||
              steps.some((s) => !s.name.trim() || !s.approver_role.trim()) ||
              save.isPending
            }
            onClick={() => save.mutate()}
          >
            {existing ? "Save changes" : "Create type"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
