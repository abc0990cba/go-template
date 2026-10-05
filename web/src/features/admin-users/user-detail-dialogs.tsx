import { useMutation, useQueryClient } from "@tanstack/react-query";

import { Check, Link2, Loader2, RefreshCw, Send } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import { Input } from "@/shared/components/ui/input";
import { Label } from "@/shared/components/ui/label";

import { getUserWithETag, updateUser } from "./api";
import { isStaleWrite } from "@/shared/api/core";

import type { User } from "@/shared/types/core";

export function EditUserDialog({
  user,
  open,
  onOpenChange,
  onSaved,
}: {
  user: User;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState(user.email);
  const [name, setName] = useState(user.name);
  const [externalRef, setExternalRef] = useState(user.external_ref ?? "");
  // Optimistic locking: the ETag captured when the dialog opens rides as
  // If-Match; a concurrent edit answers 412 and the dialog reloads.
  const [etag, setEtag] = useState("");
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    getUserWithETag(user.id)
      .then(({ etag: current }) => {
        if (!cancelled) setEtag(current);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [open, user.id]);

  const update = useMutation({
    mutationFn: () =>
      updateUser(user.id, email.trim(), name.trim(), externalRef.trim() || undefined, etag),
    onSuccess: () => {
      toast.success("User updated");
      onOpenChange(false);
      onSaved();
    },
    onError: (error: Error) => {
      if (isStaleWrite(error)) {
        toast.error(
          "The user was changed by someone else — the latest version has been loaded; please review and save again.",
        );
        void queryClient.invalidateQueries({ queryKey: ["users"] });
        getUserWithETag(user.id).then(({ data, etag: fresh }) => {
          setEtag(fresh);
          setEmail(data.email);
          setName(data.name);
          setExternalRef(data.external_ref ?? "");
        });
        return;
      }
      toast.error(error.message);
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit user</DialogTitle>
          <DialogDescription>
            Updates the account via PUT /api/users/{user.id.slice(0, 8)}… — the change is audited.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-email">Email</Label>
            <Input
              id="edit-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-ref" className="flex items-center gap-1">
              <Link2 className="size-3.5" /> External ref
            </Label>
            <Input
              id="edit-ref"
              value={externalRef}
              placeholder="correlation id for Kafka/NATS streams"
              onChange={(event) => setExternalRef(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => update.mutate()} disabled={update.isPending || !email.trim()}>
            {update.isPending ? <Loader2 className="animate-spin" /> : <Check />} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * EnrichmentCard renders one stream's owned JSONB payload, pretty-printed,
 * with a one-click test trigger: it publishes a real event into that stream
 * and the worker applies it through the normal exactly-once pipeline.
 */
export function EnrichmentCard({
  title,
  description,
  payload,
  externalRef,
  onTest,
  testing,
}: {
  title: string;
  description: string;
  payload?: string;
  externalRef?: string;
  onTest: () => void;
  testing: boolean;
}) {
  let pretty: string | null = null;
  if (payload) {
    try {
      pretty = JSON.stringify(JSON.parse(payload), null, 2);
    } catch {
      pretty = payload;
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <RefreshCw className="size-4 text-muted-foreground" /> {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="min-w-0 text-xs text-muted-foreground">
            {externalRef ? (
              <>
                external_ref <code className="font-mono">{externalRef}</code>
              </>
            ) : (
              <span>
                No external_ref — edit the user and set one, or the streams cannot find it.
              </span>
            )}
          </p>
          <Button
            size="sm"
            variant="outline"
            className="shrink-0"
            disabled={testing || !externalRef}
            onClick={onTest}
          >
            <Send /> {testing ? "Publishing…" : "Send test event"}
          </Button>
        </div>
        {pretty ? (
          <pre className="max-h-56 overflow-auto rounded-md bg-muted p-2 font-mono text-xs">
            {pretty}
          </pre>
        ) : (
          <p className="text-sm text-muted-foreground">
            No enrichment yet. Click “Send test event” to publish into this stream and watch the
            card fill in.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
