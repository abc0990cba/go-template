import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the admin-insights slice, derived from the generated API
 * contract (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape,
 * change the spec and run `npm run gen:api` — these types are never
 * hand-edited.
 */

type Schemas = components["schemas"];

export type AnalyticsSummary = Schemas["AnalyticsSummary"];

export type EventRecord = Schemas["EventRecord"];

export type EventDayPoint = Schemas["EventDayPoint"];

export type NameCount = Schemas["NameCount"];

export type DayCount = Schemas["DayCount"];

export type Report = Schemas["Report"];

export type AuditEntry = Schemas["AuditEntry"];
