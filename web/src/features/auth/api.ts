import { http, ApiError } from "@/shared/api/core";
import type { LoginOutcome, UserWithToken } from "./types";

/** Endpoints of the auth slice: registration, the two-step login, MFA
 * lifecycle, password recovery, email verification, invitation acceptance. */

export function register(email: string, name: string, password: string) {
  return http.post<UserWithToken>("/api/auth/register", { email, name, password });
}

/** loginStep classifies the password step without throwing for the two
 * intentional non-200 answers (the challenge and the enrollment gate) —
 * both arrive as 401 with a marker body (internal/auth/http.go). */
export async function loginStep(email: string, password: string): Promise<LoginOutcome> {
  let response: Response;
  try {
    response = await fetch("/api/auth/login", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
  } catch (cause) {
    throw new ApiError(
      0,
      "network error",
      cause instanceof Error ? cause.message : "network error",
    );
  }
  if (response.ok) {
    const ok = (await response.json()) as UserWithToken;
    return { kind: "ok", user: ok.user, token: ok.token };
  }
  let body: Record<string, unknown> = {};
  try {
    body = (await response.json()) as Record<string, unknown>;
  } catch {
    // Non-JSON error bodies are fine; the status still carries meaning.
  }
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

export function mfaChallenge(challenge: string, code: string) {
  return http.post<UserWithToken>("/api/auth/mfa/challenge", { challenge, code });
}

export function forgotPassword(email: string): Promise<void> {
  return http.post("/api/auth/forgot-password", { email });
}

export function resetPassword(token: string, password: string): Promise<void> {
  return http.post("/api/auth/reset-password", { token, password });
}

export function verifyEmail(token: string): Promise<void> {
  return http.post("/api/auth/verify-email", { token });
}

export function logout(): Promise<void> {
  return http.post<void>("/api/auth/logout");
}

export function acceptInvitation(token: string): Promise<{ org_id: string; role: string }> {
  return http.post("/api/invitations/accept", { token });
}
