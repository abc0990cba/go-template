## Purpose

The API contract keeps backend behavior explicit and testable.

## ADDED Requirements

### Requirement: OpenAPI Document

The system shall serve an OpenAPI document and a browser API reference.

#### Scenario: Open API docs

- **WHEN** a user opens `/docs`
- **THEN** the API reference loads from `/openapi.yaml`
