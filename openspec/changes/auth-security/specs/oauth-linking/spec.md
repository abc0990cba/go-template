## Purpose

OAuth linking defines how external provider identities attach to accounts; web security defines the browser-facing defenses around forms, cookies, and auth endpoints.

## ADDED Requirements

### Requirement: Verified Provider Linking

Provider linking shall run the OAuth2 authorization-code flow with the active user's session, and shall link identities only by the provider-verified `(provider, subject)` pair.

#### Scenario: Link google from the profile page

- **WHEN** a signed-in user follows the provider link, authorizes on the provider, and the callback exchanges the code
- **THEN** the identity appears on the profile page and the state cannot be replayed

### Requirement: Expiring Sessions

Sessions shall expire after `APP_SESSION_TTL`, renew on use, and every session of a user shall be revocable at once.

#### Scenario: Logout all

- **WHEN** the user invokes logout-all
- **THEN** every issued session token for that user stops resolving

### Requirement: CSRF Protection On Forms

Every state-changing HTML form shall require a double-submit CSRF token; the JSON API shall not.

#### Scenario: Forged form post

- **WHEN** a post arrives without the matching `csrf_token` field and cookie
- **THEN** the server responds 403 without executing the action

### Requirement: Auth Rate Limiting

Login and register shall be rate limited per IP across UI and API, rejecting overflow with 429.

#### Scenario: Burst of login attempts

- **WHEN** one client exceeds the configured burst on `/login`
- **THEN** further attempts receive 429, and the API surface enforces the same budget
