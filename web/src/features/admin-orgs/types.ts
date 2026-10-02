import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the admin-orgs slice, derived from the generated API
 * contract (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape,
 * change the spec and run `npm run gen:api` — these types are never
 * hand-edited.
 */

type Schemas = components["schemas"];

export type OrgInvitationStatus = Schemas["OrgInvitation"]["status"];

export type Organization = Schemas["Organization"];

export type OrgMember = Schemas["OrgMember"];

export type OrgInvitation = Schemas["OrgInvitation"];
