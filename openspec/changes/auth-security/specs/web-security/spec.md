## Purpose

Web security covers headers, cookie flags, and throttling behavior applied to the browser surface.

## ADDED Requirements

### Requirement: Hardening Headers

Every response shall carry Content-Security-Policy, nosniff, frame denial, and referrer policy headers.

#### Scenario: Any page

- **WHEN** a client receives any response
- **THEN** the hardening headers are present

### Requirement: Hardened Session Cookie

The session cookie shall be HttpOnly, SameSite=Lax, and Secure when `APP_COOKIE_SECURE` is enabled.

#### Scenario: Login response

- **WHEN** the login sets the session cookie
- **THEN** the cookie carries HttpOnly and SameSite=Lax attributes and honors the Secure flag
