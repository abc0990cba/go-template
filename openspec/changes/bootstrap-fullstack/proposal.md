## Why

The repository needs a reusable Go application template that is useful for training and future production projects.

It should demonstrate backend, frontend, persistence, jobs, analytics, reliability, testing, and documentation patterns without becoming too complex to understand.

## What Changes

- Add a Gin HTTP app with a browser UI.
- Add email/password auth and provider-linking scaffolds.
- Add users CRUD and pagination modes.
- Add background job and enrichment scaffolds.
- Add analytics dashboard and ClickHouse schema.
- Add OpenAPI and Scalar docs.
- Add PostgreSQL migrations, sqlc config, and query examples.
- Add reliability helpers and graceful shutdown.
- Add documentation and traceability matrices.

## Capabilities

### New Capabilities

- `user-auth`
- `user-management`
- `user-enrichment`
- `background-jobs`
- `analytics-events`
- `ui`
- `reliability`
- `api-contract`
