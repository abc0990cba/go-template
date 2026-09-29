## Purpose

The deployment capability defines how the application reaches production: a Helm chart encoding scaling, health, and security posture, and a release pipeline that refuses to publish unscanned or unsigned artifacts.

## ADDED Requirements

### Requirement: Chart With Production Posture

The chart shall deploy the api (2+ replicas, zero-downtime rolling, startup/liveness/readiness probes bound to the app's health endpoints) and the worker (metrics listener) behind a Service, with an optional Ingress, a CPU-based HPA, a PodDisruptionBudget, and a nonroot read-only security context. Postgres, Redis, ClickHouse, and OTLP endpoints shall come from values; the chart shall not manage those services.

#### Scenario: Render matches the posture

- **WHEN** the chart is templated with default values
- **THEN** the output contains the api and worker Deployments with the documented probes, an HPA targeting the api, a PDB, and nonroot security contexts

#### Scenario: External data services

- **WHEN** an operator overrides config.databaseURL and config.redisAddr
- **THEN** the rendered env carries the overrides and no in-cluster Postgres/Redis is created

### Requirement: Signed, Scanned Releases

Tag pushes shall build a multi-arch image, push it to GHCR, fail the release on unfixed CRITICAL/HIGH scan findings, attach an SPDX SBOM, and sign the image keylessly; the Helm chart shall be linted, packaged, and pushed to an OCI registry. All workflow actions shall be SHA-pinned.

#### Scenario: Tag to signed image

- **WHEN** a `v*` tag is pushed
- **THEN** the pipeline publishes a signed multi-arch image with semver tags and an attached SBOM, and pushes the packaged chart
