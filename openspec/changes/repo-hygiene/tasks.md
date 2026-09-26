## 1. Build And Toolchain

- [x] 1.1 Add `golang.org/x/oauth2` to `go.mod` and restore `go build ./...`
- [x] 1.2 Move module, Dockerfile, and docs to Go 1.26
- [x] 1.3 Add `.golangci.yml` with justified exclusions and `make lint`/`make vet`/`make coverage` targets

## 2. Error And Config Contract

- [x] 2.1 Add `service.ValidationError` and route validation, not-found, conflict, and credential errors through one HTTP mapper
- [x] 2.2 Make `config.Load` return errors; validate positivity and log level; add `.env.example`
- [x] 2.3 Log the configured level; switch `cmd/api` to `signal.NotifyContext` without `os.Exit` from the serve goroutine

## 3. HTTP Hygiene

- [x] 3.1 Request ids from `crypto/rand`, honoring upstream `X-Request-ID`
- [x] 3.2 Rename `rateLimit` to `bodyLimit`
- [x] 3.3 Strip the `Bearer ` prefix from the Authorization header
- [x] 3.4 Add snake_case JSON tags to all models
- [x] 3.5 Render client-safe errors in UI templates; log everything else; stop building job payloads by string concatenation

## 4. Tests And CI

- [x] 4.1 Table-driven tests for the memory store: conflicts, pagination modes, identities, OAuth state, jobs, analytics
- [x] 4.2 Config validation tests, middleware tests, and an httptest API flow (register/login/me/users + error mapping)
- [x] 4.3 GitHub Actions workflow: vet, build, `go test -race`, golangci-lint

## 5. Docs

- [x] 5.1 Update README, `docs/setup.md`, and `docs/development.md` (quality gates, Go 1.26, env variables)
