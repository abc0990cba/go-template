## Purpose

User authentication lets a person create an account, sign in, keep a browser session, and link multiple identities to the same account.

## ADDED Requirements

### Requirement: Email Password Registration

The system shall allow a user to register with email and password.

#### Scenario: Valid registration

- **WHEN** a visitor submits a valid email and password
- **THEN** a user account is created and a session is established

### Requirement: Provider Linking

The system shall allow an authenticated user to link Google and GitHub identities to the same user account.

#### Scenario: Link provider

- **WHEN** an authenticated user links a provider
- **THEN** the provider identity appears on the profile page
