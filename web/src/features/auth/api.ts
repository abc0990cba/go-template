import { api, ApiError, unwrap } from "@/shared/api/core";
import type { LoginOutcome, UserWithToken } from "./types";

/** Endpoints of the auth slice: registration, the two-step login, MFA
 * lifecycle, password recovery, email verification, invitation acceptance.
 * Every call goes through the typed client, so paths, bodies, and responses
 * are checked against the OpenAPI contract at compile time. */

export function register(email: string, name: string, password: string): Promise<UserWithToken> {
  return unwrap(api.POST("/api/auth/register", { body: { email, name, password } }));
}

/** loginStep classifies the password step without throwing for the two
 * intentional non-200 answers (the challenge and the enrollment gate) —
 * both arrive as 401 with a marker body (internal/auth/http.go). */
export async function loginStep(email: string, password: string): Promise<LoginOutcome> {
  const { data, error, response } = await api.POST("/api/auth/login", {
    body: { email, password },
  });
  if (response.ok && data) {
    return { kind: "ok", user: data.user, token: data.token };
  }
  // The marker fields are outside the contract's problem schema; the body is read untyped, as before.
  const body = (error ?? {}) as Record<string, unknown>;
  if (body.mfa_required === true && typeof body.challenge === "string") {
    return { kind: "mfa_required", challenge: body.challenge };
  }
  if (body.mfa_enrollment_required === true) return { kind: "enrollment_required" };
  throw new ApiError(
    response.status,
    (body.title as string) ?? "request failed",
    (body.detail as string) ?? "",
  );
}

/** login signs demo accounts straight in: the picker only offers MFA-less
 * accounts, so an mfa_required answer is a protocol violation. */
export async function login(email: string, password: string) {
  const outcome = await loginStep(email, password);
  if (outcome.kind !== "ok") {
    throw new ApiError(401, "mfa_required", "This account requires a second step");
  }
  return { user: outcome.user, token: outcome.token };
}

export function mfaChallenge(challenge: string, code: string): Promise<UserWithToken> {
  return unwrap(api.POST("/api/auth/mfa/challenge", { body: { challenge, code } }));
}

export function forgotPassword(email: string): Promise<void> {
  return unwrap(api.POST("/api/auth/forgot-password", { body: { email } }));
}

export function resetPassword(token: string, password: string): Promise<void> {
  return unwrap(api.POST("/api/auth/reset-password", { body: { token, password } }));
}

export function verifyEmail(token: string): Promise<void> {
  return unwrap(api.POST("/api/auth/verify-email", { body: { token } }));
}

export function logout(): Promise<void> {
  return unwrap(api.POST("/api/auth/logout"));
}

export function acceptInvitation(token: string): Promise<{ org_id: string; role: string }> {
  return unwrap(api.POST("/api/invitations/accept", { body: { token } }));
}
