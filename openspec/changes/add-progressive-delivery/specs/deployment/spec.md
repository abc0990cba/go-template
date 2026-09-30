## Purpose

The deployment capability delivers releases progressively: a canary gated by the same SLO metrics the alerting layer uses, with automatic rollback, GitOps handoff, and a lighter Nomad path for smaller operational budgets.

## ADDED Requirements

### Requirement: Canary With Metric Gate

When `rollout.enabled` is set, the api shall deploy as an Argo Rollouts canary that shifts 20% → 50% → 100%, running an AnalysisTemplate between steps; availability below the configured floor or p95 above the configured ceiling shall abort the rollout to stable automatically.

#### Scenario: Bad revision self-reverts

- **WHEN** a canary revision pushes 5xx ratio above the gate
- **THEN** the analysis fails, the rollout aborts, and stable serves 100% without human action

#### Scenario: Quiet canary

- **WHEN** the canary serves no traffic so the metrics carry no data
- **THEN** NaN results count as success and the rollout proceeds

### Requirement: Both Modes Render

The chart shall render the classic Deployment shape by default and the Rollout shape when enabled, with the pod spec shared so the two cannot drift; the HPA target shall follow the active kind.

#### Scenario: Render parity

- **WHEN** `helm template` runs with and without `rollout.enabled=true`
- **THEN** the pod spec is identical across both outputs modulo the workload kind

### Requirement: Deploy Handoff Is Git

An Argo CD Application shall reference the published OCI chart with automated sync and self-healing, so promotion and rollback are git operations.

#### Scenario: Promote by merge

- **WHEN** the Application's targetRevision is bumped in git
- **THEN** Argo CD applies the chart and the Rollout canaries the new revision

### Requirement: Provenance And Changelog Automation

The release pipeline shall attest SLSA build provenance for pushed images, and tagged releases shall get drafted notes grouping merged PRs by category.

#### Scenario: Verify an image

- **WHEN** a consumer runs `gh attestation verify` against a released image
- **THEN** the SLSA provenance attestation resolves to this repository's build
