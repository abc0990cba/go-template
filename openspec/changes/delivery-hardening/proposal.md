## Why

The image ran as root, carried no build metadata, and had no healthcheck; the UI rendered through `html/template` strings with no type safety.

## What Changes

- Migrate the UI from `html/template` (`views.html`) to typed templ components (`views.templ` → generated `views_templ.go`); `make generate` and the CI generated-code job cover templ.
- Harden the Dockerfile: static CGO-free binaries, `-trimpath -ldflags "-s -w"` with version/commit/date, distroless non-root runtime, HEALTHCHECK via a self `/health/live` probe (`/app/api -healthcheck`) because distroless has no shell.
- `docker build` CI job stamps the commit into the image; compose passes a version arg.

## Capabilities

### New Capabilities

- none (finishes `ui` delivery and hardening rules from reliability docs)

## Impact

- The runtime image no longer contains a shell or package manager; debugging uses ephemeral debug containers.
- Remaining open item: htmx partial updates for users list and live job status.
