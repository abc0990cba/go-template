## Purpose

User enrichment exercises bounded concurrent network calls and resilient external API access.

## ADDED Requirements

### Requirement: Batch Enrichment

The system shall enqueue a batch enrichment job that fetches public mock user data.

#### Scenario: Start enrichment

- **WHEN** an authenticated user submits a batch size
- **THEN** a background job is created for enrichment
