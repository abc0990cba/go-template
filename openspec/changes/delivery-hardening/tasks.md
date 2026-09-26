## 1. UI

- [x] 1.1 Convert all pages to templ components; render via templ.Render
- [x] 1.2 make generate + CI generated-code job cover templ

## 2. Delivery

- [x] 2.1 Static CGO-free build with -trimpath/-ldflags; version/commit/date stamped
- [x] 2.2 distroless non-root runtime; HEALTHCHECK via -healthcheck self probe
- [x] 2.3 CI docker build job; compose version arg

## 3. Open Follow-Ups

- [ ] 3.1 htmx partial updates (users list, live job status polling)
