# Quick Start

Быстрый запуск и проверка проекта. Два способа: **A — весь стек в Docker**, **Б — гибридный** (инфраструктура в контейнерах, приложение локально; на этой машине он самый надёжный — сборка образов может упереться в credential-промпт Docker, а хостовый порт 5432 занят посторонним PostgreSQL, поэтому Postgres проброшен на **55432**).

## Демо-пользователь

Если api запущен с `APP_DEMO_EMAIL` / `APP_DEMO_PASSWORD` (compose задаёт их по умолчанию), при старте автоматически создаётся аккаунт:

```
email:    demo@go-template.local
password: demo-password-123
```

Повторные запуски его не дублируют (сид идемпотентен). В продакшене эти переменные не задаются.

## Вариант Б — гибридный (рекомендуется локально)

```bash
cd "/Volumes/ADATA SC750/go-template"

# 1. Инфраструктура
docker compose up -d postgres redis

# 2. API (терминал 1); миграции применяются при старте,
#    демо-пользователь создаётся автоматически
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_COOKIE_SECURE=0 \
go run ./cmd/api

# 3. Worker (терминал 2) — фоновые задачи через Redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
go run ./cmd/worker
```

## Вариант A — весь стек в Docker

```bash
make docker-up          # api + worker + postgres + redis + clickhouse
# демо-креды уже в compose (APP_DEMO_EMAIL / APP_DEMO_PASSWORD)
```

## Проверка

| Что | Где |
|---|---|
| UI (логин демо-кредами) | http://localhost:8080/login |
| Регистрация нового | http://localhost:8080/register |
| Swagger / Scalar API docs | http://localhost:8080/docs |
| Метрики Prometheus | http://localhost:8080/metrics |
| Версия сборки | http://localhost:8080/version |
| Health live / ready / startup | /health/live, /health/ready, /health/startup |
| Jaeger UI (профиль observability) | http://localhost:16686 |
| Grafana (профиль observability) | http://localhost:3000 |

```bash
curl -fsS http://localhost:8080/health/ready   # {"status":"ok"} — БД доступна
curl -fsS http://localhost:8080/version
curl -fsS http://localhost:8080/metrics | head
curl -fsS http://localhost:8080/api/analytics/summary
```

Быстрый логин демо-пользователя через API:

```bash
curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}'
```

## Тесты

```bash
go test -tags=e2e ./test/e2e/... -count=1   # сквозной happy path по живому стеку
make test                                    # юнит-тесты с -race
make test-integration                        # контракт-тесты: память vs Postgres, Redis-очередь, ClickHouse
make coverage                                # покрытие ./internal/... (порог CI 55%, цель 70%)
make lint                                    # golangci-lint (0 issues)
```

## Данные для нагрузочных сценариев

```bash
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
go run ./cmd/seed -users 20000      # 20k пользователей за ~250мс (pgx COPY)
make perf-login                     # и другие: perf-users (MODE=offset|cursor|keyset), perf-analytics, perf-enrichment
```

## Остановить

```bash
docker compose down       # сохранить данные
docker compose down -v    # удалить тома вместе с данными
```
