import { http } from "@/shared/api/core";
import type { Paged } from "@/shared/api/pagination";
import type { SessionInfo } from "@/shared/types/core";
import type { APIKey, AuthIdentity, OAuthProviderInfo } from "./types";

/** Endpoints of the profile slice: linked identities, sessions, OAuth
 * providers, and personal API keys. */
export function myIdentities(): Promise<AuthIdentity[]> {
  return http.get<Paged<AuthIdentity>>("/api/me/identities").then((p) => p.data);
}

export function sessions(): Promise<SessionInfo[]> {
  return http.get<Paged<SessionInfo>>("/api/sessions").then((p) => p.data);
}

export function logoutAll(): Promise<void> {
  return http.post<void>("/api/profile/logout-all");
}

export function unlinkProvider(provider: string): Promise<void> {
  return http.post<void>(`/api/profile/unlink/${provider}`);
}

export function oauthProviders(): Promise<OAuthProviderInfo[]> {
  return http.get<Paged<OAuthProviderInfo>>("/api/oauth/providers").then((p) => p.data);
}

export function mfaStatus(): Promise<{ enabled: boolean; pending: boolean; required: boolean }> {
  return http.get("/api/auth/mfa/status");
}

export function mfaEnroll(): Promise<{ secret: string; otpauth_url: string }> {
  return http.post("/api/auth/mfa/enroll");
}

export function mfaVerify(code: string): Promise<{ recovery_codes: string[] }> {
  return http.post("/api/auth/mfa/verify", { code });
}

export function mfaDisable(password: string, code: string): Promise<void> {
  return http.post("/api/auth/mfa/disable", { password, code });
}

export function listAPIKeys(): Promise<APIKey[]> {
  return http.get<Paged<APIKey>>("/api/api-keys").then((p) => p.data);
}

export function createAPIKey(name: string, scopes: string[]) {
  return http.post<{ key: APIKey; secret: string }>("/api/api-keys", { name, scopes });
}

export function revokeAPIKey(id: string): Promise<void> {
  return http.del<void>(`/api/api-keys/${encodeURIComponent(id)}`);
}
