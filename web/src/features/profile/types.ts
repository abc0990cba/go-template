import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the profile slice, derived from the generated API contract
 * (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape, change the
 * spec and run `npm run gen:api` — these types are never hand-edited.
 */

type Schemas = components["schemas"];

export type AuthIdentity = Schemas["AuthIdentity"];

export type OAuthProviderInfo = Schemas["OAuthProviderInfo"];

export type APIKey = Schemas["APIKey"];
