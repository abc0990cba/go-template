## Purpose

The mfa capability adds a time-based second factor to password login: enrollment with an authenticator app, a challenge step that bridges the password and code phases, recovery codes for lost devices, a policy flag that makes the factor mandatory, and an administrative reset.

## ADDED Requirements

### Requirement: TOTP Enrollment

An authenticated user shall be able to start enrollment (receiving a base32 secret and an otpauth provisioning URI), and confirm it by submitting a valid current code. Re-enrollment while enabled is refused; disabling requires both the password and a current valid code.

#### Scenario: Enroll and confirm

- **WHEN** the user enrolls and then submits a code their authenticator generates
- **THEN** the factor is confirmed and fresh recovery codes are returned exactly once

#### Scenario: Disable needs both factors

- **WHEN** the user submits only a password, or only a code
- **THEN** the factor stays enabled

### Requirement: Challenge-Based Login

For an account with a confirmed factor, the password step shall not open a session: it shall return a short-lived challenge token, and the session shall be issued only after the challenge is answered with a valid TOTP or an unused recovery code. Challenges expire after five minutes, allow at most five wrong codes, and are deleted after use.

#### Scenario: Password step yields a challenge

- **WHEN** an enrolled account logs in with the correct password
- **THEN** the response is 401 with `mfa_required` and a challenge, and no session is created

#### Scenario: Recovery code works once

- **WHEN** the challenge is completed with a recovery code and later the same code is reused
- **THEN** the first completion opens a session and the reuse is rejected

### Requirement: Policy And Administration

The `mfa.required` setting shall block login for accounts without a confirmed factor (403 `mfa_enrollment_required`), and administrators with `users:update` shall be able to reset any user's factor (idempotent, audited, with an in-app notification).

#### Scenario: Policy blocks unenrolled accounts

- **WHEN** `mfa.required` is on and an account without a confirmed factor logs in with valid credentials
- **THEN** the response is 403 with the enrollment-required marker, while wrong passwords keep reporting invalid credentials

#### Scenario: Admin reset

- **WHEN** an administrator resets a user's factor
- **THEN** the factor and recovery codes are gone and the user can sign in with the password alone and re-enroll
