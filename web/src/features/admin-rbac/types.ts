import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the admin-rbac slice, derived from the generated API
 * contract (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape,
 * change the spec and run `npm run gen:api` — these types are never
 * hand-edited.
 */

type Schemas = components["schemas"];

export type Role = Schemas["Role"];

/** Casbin ptype "p": subject/domain/object/action; "g": user/role/domain. */
export type PolicyRule = Schemas["PolicyRule"];

export type RbacResource = Schemas["RbacResource"];

export type Permission = Schemas["Permission"];
