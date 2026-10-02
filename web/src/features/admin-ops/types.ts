import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the admin-ops slice, derived from the generated API
 * contract (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape,
 * change the spec and run `npm run gen:api` — these types are never
 * hand-edited.
 */

type Schemas = components["schemas"];

export type JobStatus = Schemas["Job"]["status"];

/** The job type is a free-form string in the spec (the backend registers
 * job kinds independently of this contract), so the known kinds stay
 * handwritten. */
export type JobType = "user.enrich_batch" | "user.report_generate";

export type Job = Schemas["Job"];

export type EnrichmentProfile = Schemas["EnrichmentProfile"];

export type OutboxEvent = Schemas["OutboxEvent"];

export type DeadLetter = Schemas["DeadLetter"];

export type WebhookEndpoint = Schemas["WebhookEndpoint"];

export type WebhookDelivery = Schemas["WebhookDelivery"];
