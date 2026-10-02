import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";

/** Endpoints of the admin-ops slice: background jobs, enrichment streams,
 * the outbox/dead-letter console, and webhooks. */
export function listJobs(params: Partial<ListParams>) {
  return http.get<Paged<Job>>(`/api/jobs?${buildListQuery(params)}`);
}

export function getJob(id: string) {
  return http.get<Job>(`/api/jobs/${encodeURIComponent(id)}`);
}

export function processJob() {
  return http.post<{ processed: boolean }>("/api/jobs/process");
}

export function enqueueEnrichment(count: number) {
  return http.post<Job>("/api/jobs/enrichment", { count });
}

export function listEnrichmentProfiles(params: Partial<ListParams>) {
  return http.get<Paged<EnrichmentProfile>>(`/api/enrichment-profiles?${buildListQuery(params)}`);
}

export function listOutbox(params: Partial<ListParams> & { status?: string }) {
  return http.get<Paged<OutboxEvent> & { pending?: number }>(
    `/api/streams/outbox?${buildListQuery(params)}`,
  );
}

export function listDeadLetters(params: Partial<ListParams> & { pending?: boolean }) {
  return http.get<Paged<DeadLetter> & { pending?: number }>(
    `/api/streams/dead-letters?${buildListQuery({ ...params, status: params.pending ? "pending" : undefined })}`,
  );
}

export function replayDeadLetter(id: string) {
  return http.post<DeadLetter>(`/api/streams/dead-letters/${encodeURIComponent(id)}/replay`, {});
}

export function listWebhooks() {
  return http.get<Paged<WebhookEndpoint>>("/api/webhooks").then((p) => p.data);
}

export function createWebhook(url: string, events: string[]) {
  return http.post<{ id: string; url: string; secret: string; events: string[]; active: boolean }>(
    "/api/webhooks",
    { url, events },
  );
}

export function updateWebhook(
  id: string,
  url: string,
  events: string[],
  description: string,
  active: boolean,
) {
  return http.put<void>(`/api/webhooks/${encodeURIComponent(id)}`, {
    url,
    events,
    description,
    active,
  });
}

export function deleteWebhook(id: string) {
  return http.del<void>(`/api/webhooks/${encodeURIComponent(id)}`);
}

export function webhookDeliveries(id: string) {
  return http
    .get<Paged<WebhookDelivery>>(`/api/webhooks/${encodeURIComponent(id)}/deliveries`)
    .then((p) => p.data);
}

export function testWebhook(id: string) {
  return http.post<{ status: string }>(`/api/webhooks/${encodeURIComponent(id)}/test`);
}

import type {
  DeadLetter,
  EnrichmentProfile,
  Job,
  OutboxEvent,
  WebhookDelivery,
  WebhookEndpoint,
} from "./types";
