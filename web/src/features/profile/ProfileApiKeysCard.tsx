import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { createAPIKey, listAPIKeys, revokeAPIKey } from "./api";

export function APIKeysCard() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState("");
  const [secret, setSecret] = useState<string | null>(null);
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listAPIKeys });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["api-keys"] });

  const create = useMutation({
    mutationFn: () =>
      createAPIKey(
        name,
        scopes
          .split(/[\s,]+/)
          .map((scope: string) => scope.trim())
          .filter(Boolean),
      ),
    onSuccess: ({ secret: raw }) => {
      setSecret(raw);
      setName("");
      setScopes("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => revokeAPIKey(id),
    onSuccess: () => {
      toast.success("Key revoked");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>API keys</CardTitle>
        <CardDescription>
          Programmatic access with your permissions narrowed by scopes (e.g. users:read, jobs:*).
          Send the key as an Authorization: Bearer header.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {secret && (
          <div className="rounded border border-emerald-500/40 bg-emerald-500/10 p-2 text-xs">
            <p className="font-medium">Copy your key now — it is shown only once:</p>
            <code className="break-all font-mono">{secret}</code>
          </div>
        )}
        <div className="flex flex-wrap gap-2">
          <Input
            className="w-48"
            placeholder="Key name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Input
            className="w-64"
            placeholder="scopes: users:read, jobs:*"
            value={scopes}
            onChange={(e) => setScopes(e.target.value)}
          />
          <Button size="sm" onClick={() => create.mutate()} disabled={!name || create.isPending}>
            Create key
          </Button>
        </div>
        <div className="space-y-1">
          {(keys.data ?? []).map((key) => (
            <div
              key={key.id}
              className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
            >
              <span className="font-medium">{key.name}</span>
              <code className="text-xs text-muted-foreground">{key.prefix}…</code>
              {key.revoked_at ? (
                <Badge variant="secondary">revoked</Badge>
              ) : (
                <Badge variant="outline">active</Badge>
              )}
              <span className="text-xs text-muted-foreground">{key.scopes.join(", ")}</span>
              {!key.revoked_at && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="ml-auto size-6 text-destructive"
                  aria-label={`Revoke ${key.name}`}
                  onClick={() => revoke.mutate(key.id)}
                >
                  <Trash2 />
                </Button>
              )}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
