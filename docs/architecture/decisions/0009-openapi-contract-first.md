# ADR 0009 — OpenAPI Contract-First with Types-Only Codegen

**Status:** Accepted

## Context

An API template needs a single source of truth for request/response
shapes, visible documentation, and a way to keep Go handlers and the
TypeScript client honest as both evolve. Full server-interface generation
would couple the router to generated code and fight hand-written
middleware; no generation at all lets the spec rot.

## Decision

`api/openapi/openapi.yaml` (OpenAPI 3.1, 115 operations) is the **source
of truth**. Codegen is **types-only**: oapi-codegen emits wire structs to
`internal/api/api.gen.go`; handlers remain hand-written Gin code that
binds to those types. CI builds the image (which compiles against the
generated types), and `make generate` regenerates on demand. The spec is
served live at `/openapi.yaml` with a Scalar UI at `/docs`.

## Consequences

**Positive:** contract review happens in YAML diffs (the right review
surface); no drift between docs and behavior (docs *are* the file being
reviewed); middleware stays first-class Gin; frontend types are
hand-maintained but validated against a spec that cannot silently rot
because generated Go code fails to compile when the YAML changes shape.

**Negative:** handler-level type safety for responses is weaker than
full `ServerInterface` generation (binding is checked, response
marshalling is by convention); the TS side is manual — acceptable at
this size, replaceable with `openapi-typescript` if it grows.

## Alternatives considered

- *oapi-codegen ServerInterface/chi-runtime generation* — maximal
  safety, but middleware composition and SSE/SPA edge cases fight the
  generator; types-only keeps the HTTP layer idiomatic.
- *Spec-after-implementation* — the spec always lies eventually.
- *GraphQL* — different contract problem; not the template's goal.
