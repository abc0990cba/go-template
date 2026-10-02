import type { components } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the auth slice, derived from the generated API contract
 * (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape, change the
 * spec and run `npm run gen:api` — these types are never hand-edited.
 */

type Schemas = components["schemas"];

/** The session the register, login, and mfa-challenge endpoints hand back. */
export type UserWithToken = Schemas["UserWithToken"];

/**
 * MFA answers the login can hand back instead of a session. The two
 * non-"ok" kinds are 401 marker bodies (internal/auth/http.go) that the
 * contract does not model, so this union stays handwritten.
 */
export type LoginOutcome =
  | { kind: "ok"; user: UserWithToken["user"]; token: string }
  | { kind: "mfa_required"; challenge: string }
  | { kind: "enrollment_required" };
