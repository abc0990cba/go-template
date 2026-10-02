import type { User } from "@/shared/types/core";

interface UserWithToken {
  user: User;
  token: string;
}

/** MFA answers the login can hand back instead of a session. */
export type LoginOutcome =
  | { kind: "ok"; user: User; token: string }
  | { kind: "mfa_required"; challenge: string }
  | { kind: "enrollment_required" };

export type { UserWithToken };
