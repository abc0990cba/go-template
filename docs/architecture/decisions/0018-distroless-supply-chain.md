# ADR 0018 — Distroless Images + Scanned, Signed Supply Chain

**Status:** Accepted

## Context

Container defaults (Debian/Alpine + shell) carry a CVE surface and an
implicit "debug by exec" habit; release pipelines that only push images
leave provenance and integrity as trust-me claims. A production-shaped
template should show the 2020s baseline: minimal runtime, verified
artifacts.

## Decision

**Runtime:** `gcr.io/distroless/static-debian12:nonroot` — static
binaries (`CGO_ENABLED=0`), no shell, no package manager, uid 65532;
healthcheck implemented as an api binary flag (`-healthcheck` self-probes
`/health/live`) because there is no curl in the image. Helm adds
`readOnlyRootFilesystem`, dropped capabilities.

**Release (tag `v*`):** buildx multi-arch (amd64+arm64) → GHCR; **Trivy**
scan failing on CRITICAL/HIGH (unfixed vulns ignored) with SARIF to
GitHub code scanning; **Syft** SPDX SBOM attached; **cosign keyless**
signing via the workflow's OIDC identity; Helm chart packaged and pushed
to `oci://ghcr.io/.../charts`. All workflow Actions are SHA-pinned.

## Consequences

**Positive:** minimal attack surface; verifiable provenance for every
release (verify with `cosign verify`); scanning gates releases instead of
generating ignored reports; SBOM gives consumers an inventory.

**Negative:** no shell means debugging is `docker debug`/ephemeral
containers or restart with a debug image — a habit change; keyless cosign
verifies via OIDC identity, so consumers must record the workflow
identity in their verification policy; distroless + `go:embed` forces the
SPA to exist before the Go build (handled in the Dockerfile stage order).

## Alternatives considered

- *Alpine runtime* — smaller delta from habit, but still a shell+libc
  surface (and musl quirks); static + distroless dominates it.
- *Skip signing/SBOM at template scale* — exactly the debt that is
  expensive to add later; the pipeline is the template.
- *SLSA provenance generation (official)* — the natural next step; cosign
  keyless already carries most of the value.
