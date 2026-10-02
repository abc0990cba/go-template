import { api, unwrap } from "@/shared/api/core";
import type { ListParams, Paged } from "@/shared/api/pagination";
import type { operations } from "@/shared/api/schema.gen";
import type {
  DeadLetter,
  EnrichmentProfile,
  Job,
  OutboxEvent,
  WebhookDelivery,
  WebhookEndpoint,
} from "./types";

/** Endpoints of the admin-ops slice: background jobs, enrichment profiles,
 * the outbox/dead-letter console, and webhooks. Every call goes through the
 * typed client, so paths, bodies, and responses are checked against the
 * OpenAPI contract at compile time. */

type ListJobsQuery = operations["listJobs"]["parameters"]["query"];

/** One outbox/dead-letters page: the list envelope plus the pending count
 * the page chips render. */
type OutboxPage = Paged<OutboxEvent> & { pending: number };
type DeadLetterPage = Paged<DeadLetter> & { pending: number };

/** The created-endpoint body: the one-time secret comes back only here. */
type CreatedWebhook = operations["createWebhook"]["responses"][201]["content"]["application/json"];

/** The shared list vocabulary. The URL keeps status as a free string; the
 * backend 400s anything outside the spec enum, and the table filter only
 * offers valid values. */
export function listJobs(params: Partial<ListParams>): Promise<Paged<Job>> {
  return unwrap(api.GET("/api/jobs", { params: { query: params as ListJobsQuery } }));
}

export function getJob(id: string): Promise<Job> {
  return unwrap(api.GET("/api/jobs/{id}", { params: { path: { id } } }));
}

export function processJob(): Promise<{ processed: boolean }> {
  return unwrap(api.POST("/api/jobs/process"));
}

export function enqueueEnrichment(count: number): Promise<Job> {
  return unwrap(api.POST("/api/jobs/enrichment", { body: { count } }));
}

export function listEnrichmentProfiles(
  params: Partial<ListParams>,
): Promise<Paged<EnrichmentProfile>> {
  /** The spec vocabulary for profiles: the shared list params plus the
   * user_id filter they add. */
  return unwrap(
    api.GET("/api/enrichment-profiles", {
      params: {
        query: {
          mode: params.mode,
          limit: params.limit,
          offset: params.offset,
          after: params.after,
          q: params.q,
          user_id: params.user_id,
        },
      },
    }),
  );
}

/** The streams console: the transactional outbox, parked dead letters, and
 * replay — all spec-covered (listOutbox, listDeadLetters, replayDeadLetter). */

export function listOutbox(
  params: Partial<ListParams> & { status?: "pending" },
): Promise<OutboxPage> {
  type Query = operations["listOutbox"]["parameters"]["query"];
  const { status, ...rest } = params;
  return unwrap(
    api.GET("/api/streams/outbox", {
      params: { query: { ...rest, status } as Query },
    }),
  ) as Promise<OutboxPage>;
}

export function listDeadLetters(
  params: Partial<ListParams> & { pending?: boolean },
): Promise<DeadLetterPage> {
  type Query = operations["listDeadLetters"]["parameters"]["query"];
  const { pending, ...rest } = params;
  return unwrap(
    api.GET("/api/streams/dead-letters", {
      params: { query: { ...rest, pending: pending ? "true" : undefined } as Query },
    }),
  ) as Promise<DeadLetterPage>;
}

export function replayDeadLetter(id: string): Promise<DeadLetter> {
  return unwrap(api.POST("/api/streams/dead-letters/{id}/replay", { params: { path: { id } } }));
}

export function listWebhooks(): Promise<WebhookEndpoint[]> {
  return unwrap(api.GET("/api/webhooks")).then((p) => p.data);
}

export function createWebhook(url: string, events: string[]): Promise<CreatedWebhook> {
  return unwrap(api.POST("/api/webhooks", { body: { url, events } }));
}

export function updateWebhook(
  id: string,
  url: string,
  events: string[],
  description: string,
  active: boolean,
): Promise<void> {
  return unwrap(
    api.PUT("/api/webhooks/{id}", {
      params: { path: { id } },
      body: { url, events, description, active },
    }),
  );
}

export function deleteWebhook(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/webhooks/{id}", { params: { path: { id } } }));
}

export function webhookDeliveries(id: string): Promise<WebhookDelivery[]> {
  return unwrap(api.GET("/api/webhooks/{id}/deliveries", { params: { path: { id } } })).then(
    (p) => p.data,
  );
}

export function testWebhook(id: string): Promise<{ status?: string }> {
  return unwrap(api.POST("/api/webhooks/{id}/test", { params: { path: { id } } }));
}
