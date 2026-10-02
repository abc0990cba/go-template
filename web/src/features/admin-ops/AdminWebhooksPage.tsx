import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format } from "date-fns";
import { useState } from "react";
import { toast } from "sonner";
import { Loader2, Plus, Send, Trash2 } from "lucide-react";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { Badge } from "@/shared/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import { Button } from "@/shared/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/components/ui/card";
import { Input } from "@/shared/components/ui/input";
import { Switch } from "@/shared/components/ui/switch";
import {
  createWebhook,
  deleteWebhook,
  listWebhooks,
  testWebhook,
  updateWebhook,
  webhookDeliveries,
} from "./api";
import type { WebhookDelivery, WebhookEndpoint } from "./types";

export function AdminWebhooksPage() {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const endpoints = useQuery({ queryKey: ["webhooks"], queryFn: listWebhooks });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["webhooks"] });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Webhooks</h1>
          <p className="text-sm text-muted-foreground">
            Push platform events to your systems — signed with HMAC-SHA256.
          </p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus /> New endpoint
        </Button>
      </div>
      <ExplainerBanner
        title="Webhooks — events pushed to you"
        howTo={[
          'Subscribe to event names ("*" for everything); deliveries fire on the analytics event bus.',
          "Every POST carries X-Template-Signature: sha256=HMAC(secret, body) — verify before trusting.",
          "The delivery log records every attempt; use Send test to verify an endpoint instantly.",
        ]}
      >
        Deliveries execute through the job queue when Redis is configured and inline otherwise;
        failed attempts retry with backoff and each round lands in the log below.
      </ExplainerBanner>

      <div className="grid gap-3 md:grid-cols-2">
        {(endpoints.data ?? []).map((endpoint) => (
          <EndpointCard key={endpoint.id} endpoint={endpoint} onChanged={invalidate} />
        ))}
        {!endpoints.isPending && (endpoints.data ?? []).length === 0 && (
          <p className="text-sm text-muted-foreground">No endpoints yet.</p>
        )}
      </div>

      <CreateWebhookDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(secret) => {
          invalidate();
          if (secret)
            toast.info("Endpoint secret (shown once)", { description: secret, duration: 15000 });
        }}
      />
    </div>
  );
}

function EndpointCard({
  endpoint,
  onChanged,
}: {
  endpoint: WebhookEndpoint;
  onChanged: () => void;
}) {
  const deliveries = useQuery({
    queryKey: ["webhooks", endpoint.id, "deliveries"],
    queryFn: () => webhookDeliveries(endpoint.id),
  });
  const remove = useMutation({
    mutationFn: () => deleteWebhook(endpoint.id),
    onSuccess: () => {
      toast.success("Endpoint deleted");
      onChanged();
    },
    onError: (error) => toast.error(error.message),
  });
  const test = useMutation({
    mutationFn: () => testWebhook(endpoint.id),
    onSuccess: () => {
      toast.success("Test event queued");
      setTimeout(onChanged, 1500);
    },
    onError: (error) => toast.error(error.message),
  });
  const toggle = useMutation({
    mutationFn: (active: boolean) =>
      updateWebhook(endpoint.id, endpoint.url, endpoint.events, endpoint.description ?? "", active),
    onSuccess: onChanged,
    onError: (error) => toast.error(error.message),
  });

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <code className="truncate">{endpoint.url}</code>
          <Switch
            checked={endpoint.active}
            onCheckedChange={(checked) => toggle.mutate(checked)}
            aria-label="Toggle endpoint"
          />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex flex-wrap gap-1">
          {endpoint.events.map((event) => (
            <Badge key={event} variant="outline" className="font-mono text-[10px]">
              {event}
            </Badge>
          ))}
        </div>
        <div className="max-h-40 space-y-1 overflow-y-auto">
          {(deliveries.data ?? []).slice(0, 8).map((delivery) => (
            <DeliveryRow key={delivery.id} delivery={delivery} />
          ))}
          {!deliveries.isPending && (deliveries.data ?? []).length === 0 && (
            <p className="text-xs text-muted-foreground">No deliveries yet.</p>
          )}
        </div>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={() => test.mutate()}
            disabled={test.isPending}
          >
            <Send /> Send test
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="text-destructive"
            onClick={() => remove.mutate()}
            disabled={remove.isPending}
          >
            <Trash2 />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function DeliveryRow({ delivery }: { delivery: WebhookDelivery }) {
  return (
    <div className="flex items-center gap-2 rounded border px-2 py-1 font-mono text-[11px]">
      <Badge
        variant={
          delivery.status === "success"
            ? "default"
            : delivery.status === "pending"
              ? "secondary"
              : "destructive"
        }
      >
        {delivery.status}
      </Badge>
      <span className="truncate">{delivery.event}</span>
      <span className="ml-auto text-muted-foreground">
        {delivery.attempts}×{" "}
        {delivery.last_response_status ? `· ${delivery.last_response_status}` : ""}
        {" · "}
        {format(new Date(delivery.created_at), "HH:mm:ss")}
      </span>
    </div>
  );
}

function CreateWebhookDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (secret: string) => void;
}) {
  void open;
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState("user.logged_in, job.done");
  const create = useMutation({
    mutationFn: () =>
      createWebhook(
        url,
        events
          .split(",")
          .map((e) => e.trim())
          .filter(Boolean),
      ),
    onSuccess: ({ secret }) => {
      onOpenChange(false);
      onCreated(secret ?? "");
      setUrl("");
      setEvents("user.logged_in, job.done");
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>New webhook endpoint</DialogTitle>
          <DialogDescription>The signing secret is generated and shown once.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            placeholder="https://example.com/hooks/app"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <Input
            value={events}
            onChange={(e) => setEvents(e.target.value)}
            placeholder="Event names, comma separated"
          />
          <p className="text-xs text-muted-foreground">
            Events: user.registered, user.logged_in, oauth_linked, job_queued, job_done, job_failed,
            webhook.test — or * for everything.
          </p>
          <Button
            className="w-full"
            onClick={() => create.mutate()}
            disabled={!url || create.isPending}
          >
            {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Create
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
