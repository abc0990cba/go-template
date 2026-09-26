## Purpose

Engineering experience defines how the repository guarantees code health: every build green, every lint finding resolved or justified, configuration validated at startup, and one explicit mapping from domain errors to HTTP responses.

## ADDED Requirements

### Requirement: Green Baseline

The system shall build, pass `go vet`, pass race-enabled tests, and pass golangci-lint with zero findings at every commit.

#### Scenario: CI on push

- **WHEN** a commit is pushed to `main` or a pull request is opened
- **THEN** GitHub Actions runs vet, build, `go test -race ./...`, and golangci-lint, and the change is rejected on any failure

### Requirement: Fail-Fast Configuration

The system shall reject invalid configuration at startup instead of silently falling back.

#### Scenario: Typo in a numeric variable

- **WHEN** `APP_MAX_CONCURRENCY=eight` is set
- **THEN** the binary exits with an error naming the variable, and the default value is not used

### Requirement: Explicit Error Contract

The system shall map domain errors to HTTP statuses in one place and log unexpected errors without exposing their messages.

#### Scenario: Duplicate email over the API

- **WHEN** a client registers two users with the same email
- **THEN** the second request receives `409` with a `problem+json` body, and no internal detail is added beyond the safe conflict message

### Requirement: Honest UI Error Handling

The system shall never discard handler errors: client-safe messages render in the page, unexpected errors are logged.

#### Scenario: Failed form action in the UI

- **WHEN** a form action returns a validation or conflict error
- **THEN** the page re-renders with the message, and unexpected failures are logged with their cause
