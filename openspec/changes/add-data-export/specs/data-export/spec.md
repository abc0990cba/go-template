## Purpose

The data-export capability delivers self-service data portability: a user requests everything the system holds about them, a worker assembles a ZIP of per-entity JSON documents, and the user downloads their own bundle. Ownership is absolute; retention is bounded.

## ADDED Requirements

### Requirement: Export Request Lifecycle

A signed-in user shall request an export, learn its status, and download the finished bundle. Requests enqueue a real queue job (visible on the jobs page); a second request while one is queued or running answers 409; without a queue backend the request answers 400 naming the missing dependency. While the export runs, the download endpoint answers 202 with `Retry-After`.

#### Scenario: Duplicate requests conflict

- **WHEN** the user requests an export twice before the first finishes
- **THEN** the second request answers 409 and only one job exists

#### Scenario: Poll until ready

- **WHEN** the user polls the download endpoint while the job runs
- **THEN** it answers 202 with Retry-After until the bundle is ready, then 200

### Requirement: Complete, Owner-Only Bundles

The bundle shall include the profile, linked identities, session metadata, owned tickets, notifications, audit entries where the user is the subject, and file metadata — one JSON document per entity inside a single ZIP. Only the owner shall be able to resolve or download a bundle; this owner-only rule shall also apply to every non-ticket file. Bundles expire after 7 days and the worker prunes expired metadata and blobs.

#### Scenario: Bundle contents

- **WHEN** an export completes for a user with tickets and notifications
- **THEN** the ZIP contains the corresponding JSON documents and the profile

#### Scenario: Ownership is absolute

- **WHEN** another authenticated user tries to download or resolve someone's bundle
- **THEN** the answer is not-found semantics at both the export and files layers
