# ADR 0019 — OpenSpec Change-First Workflow with Traceability Matrices

**Status:** Accepted

## Context

Templates die of entropy: features get added ad hoc, docs drift, tests
don't cover what the README promises. The repo needed a lightweight,
enforceable definition of "this feature is done" that a small team (or a
coding agent) can follow without a PM tool.

## Decision

Every feature starts as an **OpenSpec change**:
`openspec/changes/<name>/{proposal.md, specs/<capability>/spec.md,
tasks.md}` — a proposal (why), capability specs (what the behavior *is*),
and a task checklist. The repo rule (README "Project Rules"): no feature
complete until the change exists, OpenAPI updated for HTTP behavior,
migrations + sqlc queries for DB behavior, tests/perf added, and the
traceability matrices updated:

- `docs/testing/requirements-matrix.md` — requirement → where implemented
- `docs/testing/traceability-matrix.md` — requirement → spec → tests → docs
- `docs/testing/test-matrix.md` / `docs/testing/performance-matrix.md` — coverage and
  budgets per feature

`make openspec-validate` checks structure when the CLI is available. The
21 existing change folders double as the project's decision/design
history alongside the ADRs.

## Consequences

**Positive:** the paper trail is in-repo, diffable, and reviewable with
the code; onboarding means reading specs next to implementations; AI/agent
contributors get an unambiguous workflow; matrices make "docs vs reality"
drift a visible CI-adjacent chore instead of archaeology.

**Negative:** ceremony for tiny changes (a one-line bugfix still doesn't
need a change folder — the rule targets *features*); matrices are
hand-maintained and can lie if skipped in review (mitigated by the
regression checklist, which verifies behavior live).

## Alternatives considered

- *External issue tracker only* — loses the in-repo, code-adjacent trail.
- *ADRs for everything* — ADRs capture *decisions*, OpenSpec captures
  *behavior specs and task flow*; they compose (hence ADR 0019).
- *RFC directories / C4-Toolbox* — heavier than a template needs.
