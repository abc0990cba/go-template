## Purpose

Persistence defines how domain state survives restarts and is shared between processes: one contract, two adapters, and SQL that matches the documented behavior.

## ADDED Requirements

### Requirement: PostgreSQL Adapter Behind Ports

The system shall implement every service port on PostgreSQL (pgx + sqlc) and select it whenever `APP_DATABASE_URL` is configured.

#### Scenario: API with database URL

- **WHEN** the api starts with `APP_DATABASE_URL` set
- **THEN** it applies pending migrations, serves traffic on PostgreSQL, and data survives a restart

### Requirement: Single Store Contract

The in-memory fake and the PostgreSQL adapter shall behave identically for every operation in the contract suite (conflicts, pagination modes, identities, sessions, OAuth state, jobs, analytics).

#### Scenario: Contract suite

- **WHEN** `make test-integration` runs
- **THEN** the same scenarios pass against both adapters, including a real PostgreSQL container

### Requirement: Honest Readiness

`/health/ready` shall reflect database reachability rather than return a constant.

#### Scenario: Database unreachable

- **WHEN** PostgreSQL is stopped while the api is running
- **THEN** `/health/ready` responds 503 while `/health/live` stays 200
