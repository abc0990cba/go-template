import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the admin-settings slice, derived from the generated API
 * contract (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape,
 * change the spec and run `npm run gen:api` — these types are never
 * hand-edited.
 */

type Schemas = components["schemas"];

export type SettingDefinition = Schemas["SettingDefinition"];

/** A catalog entry joined with its current value and audit trail. */
export type SettingView = Schemas["Setting"];
