## Purpose

Contract sync keeps `api/openapi/openapi.yaml` and the served API from drifting apart: generated types, documented operations, and CI enforcement.

## ADDED Requirements

### Requirement: Complete Contract

Every served `/api` operation shall be documented with request and response schemas, and every protected operation shall declare its security requirement.

#### Scenario: Contract review

- **WHEN** an endpoint is added or changed
- **THEN** `api/openapi/openapi.yaml` describes it with schemas in the same change

### Requirement: Generated Wire Types

Request binding shall use oapi-codegen-generated types, and CI shall reject any commit where regeneration produces a diff.

#### Scenario: Contract drift

- **WHEN** the YAML and the generated Go types disagree
- **THEN** the CI generated-code job fails the build
