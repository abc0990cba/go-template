import { api, unwrap } from "@/shared/api/core";
import type { SessionInfo } from "@/shared/types/core";
import type { operations } from "@/shared/api/schema.gen";
import type { APIKey, AuthIdentity, OAuthProviderInfo } from "./types";

/** Endpoints of the profile slice: linked identities, sessions, OAuth
 * providers, and personal API keys. Every call goes through the typed
 * client, so paths, bodies, and responses are checked against the OpenAPI
 * contract at compile time. */

export function myIdentities(): Promise<AuthIdentity[]> {
  return unwrap(api.GET("/api/me/identities")).then((p) => p.data);
}

/** SessionInfo keeps its shared/types identity: the spec schema is the same
 * shape, so the generated list assigns to it directly. */
export function sessions(): Promise<SessionInfo[]> {
  return unwrap(api.GET("/api/sessions")).then((p) => p.data);
}

export function logoutAll(): Promise<void> {
  return unwrap(api.POST("/api/profile/logout-all"));
}

/** The spec path accepts the four OAuth providers only — "password" is a
 * credential row the UI never offers to unlink. */
export function unlinkProvider(
  provider: operations["unlinkProvider"]["parameters"]["path"]["provider"],
): Promise<void> {
  return unwrap(api.POST("/api/profile/unlink/{provider}", { params: { path: { provider } } }));
}

export function oauthProviders(): Promise<OAuthProviderInfo[]> {
  return unwrap(api.GET("/api/oauth/providers")).then((p) => p.data);
}

export function mfaStatus() {
  return unwrap(api.GET("/api/auth/mfa/status"));
}

export function mfaEnroll() {
  return unwrap(api.POST("/api/auth/mfa/enroll"));
}

export function mfaVerify(code: string) {
  return unwrap(api.POST("/api/auth/mfa/verify", { body: { code } }));
}

export function mfaDisable(password: string, code: string): Promise<void> {
  return unwrap(api.POST("/api/auth/mfa/disable", { body: { password, code } }));
}

export function listAPIKeys(): Promise<APIKey[]> {
  return unwrap(api.GET("/api/api-keys")).then((p) => p.data);
}

export function createAPIKey(name: string, scopes: string[]) {
  return unwrap(api.POST("/api/api-keys", { body: { name, scopes } }));
}

export function revokeAPIKey(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/api-keys/{id}", { params: { path: { id } } }));
}
