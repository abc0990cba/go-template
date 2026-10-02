import type { components } from "@/shared/api/schema.gen";

/**
 * Entities every layer talks about: the account, the session probe.
 * Derived from the generated API contract (schema.gen.ts ←
 * api/openapi/openapi.yaml); to change a shape, change the spec and run
 * `npm run gen:api`.
 */

type Schemas = components["schemas"];

export type User = Schemas["User"];

/** GET /api/me: the account plus the RBAC context the UI gates on. */
export type Me = Schemas["Me"];

export type SessionInfo = Schemas["SessionInfo"];
