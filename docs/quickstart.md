# Quick Start

Быстрый запуск и проверка проекта. Два способа: **A — весь стек в Docker**, **Б — гибридный** (инфраструктура в контейнерах, приложение локально; на этой машине он самый надёжный — сборка образов может упереться в credential-промпт Docker, а хостовый порт 5432 занят посторонним PostgreSQL, поэтому Postgres проброшен на **55432**).

## Демо-доступ

Если api запущен с `APP_DEMO_EMAIL` / `APP_DEMO_PASSWORD` (compose задаёт их по умолчанию), при старте автоматически создаётся аккаунт:

```
email:    demo@go-template.local
password: demo-password-123
```

Повторные запуски его не дублируют (сид идемпотентен). В продакшене эти переменные не задаются.

## Демо-данные (SPA-демо)

`APP_DEMO_SEED=1` (включено в compose) наполняет базу демонстрационным набором — идемпотентно, только если данных ещё нет:

- ~500 пользователей за последние 90 дней; первые десять (`demo-u-0001@demo.example` … `demo-u-0010@demo.example`, пароль `password123`) умеют логиниться;
- 3 организации (`demo-org-1` Acme Corp, `demo-org-2` Globex, `demo-org-3` Initech) с участниками и ролями (менеджеры, вьюеры);
- роли платформы: `demo-u-0001` — admin, `demo-u-0002` — manager, `demo-u-0003` — viewer;
- ~200 записей журнала аудита, ~60 уведомлений (часть непрочитанных — колокольчик живой), 2 API-ключа, неактивный вебхук-эндпоинт с историей доставок;
- 90 дней событий (12 типов) с заполненными properties (feature/source/country) — питают графики DAU, разбивки по фичам и странам;
- ~45 OAuth-идентичностей (google/github);
- 80 фоновых задач во всех статусах (queued/running/done/failed) с правдоподобными ошибками;
- 22 отчёта-снапшота, ~110 enrichment-профилей;
- 30 дней истории событий в ClickHouse (или в in-memory трейле) — графики, лента и live-фид сразу живые.

Ручное управление:

```bash
make seed-demo          # досыпать данные, если их нет
make seed-demo-reset    # стереть demo-* данные и засеять заново
```

## Вариант Б — гибридный (рекомендуется локально)

```bash
cd "/Volumes/ADATA SC750/go-template"

# 1. Инфраструктура
docker compose up -d postgres redis clickhouse

# 2. API (терминал 1); миграции применяются при старте,
#    демо-пользователь и демо-данные создаются автоматически
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_CLICKHOUSE_ADDR="127.0.0.1:9000" \
APP_CLICKHOUSE_USER="app" APP_CLICKHOUSE_PASSWORD="app" \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_DEMO_SEED=1 \
APP_COOKIE_SECURE=0 \
go run ./cmd/api

# 3. Worker (терминал 2) — фоновые задачи через Redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_CLICKHOUSE_ADDR="127.0.0.1:9000" \
APP_CLICKHOUSE_USER="app" APP_CLICKHOUSE_PASSWORD="app" \
go run ./cmd/worker

# 4. Фронтенд (терминал 3) — Vite dev-сервер с прокси на :8080
cd web && npm ci && npm run dev
# → http://localhost:5173
```

Без запущенного Vite api всё равно отдаёт интерфейс: `make web-build` единожды собирает SPA в `web/dist`, и бинарь раздаёт его сам (`go:embed`).

## Вариант A — весь стек в Docker

```bash
make docker-up
# → http://localhost:8080  (SPA + API + SSE)
```

compose по умолчанию включает демо-пользователя и `APP_DEMO_SEED=1`, так что после старта в приложении уже есть пользователи, задачи, отчёты и история событий.

## Что проверять в браузере

| URL | Что это |
| --- | --- |
| `/roles`, `/policies`, `/orgs` | RBAC: матрица прав, raw-политики, организации (см. docs/rbac.md) |
| `/audit` | Журнал аудита: кто/что/когда, фильтры, diff |
| `/settings` | Настройки и feature flags (например, выключите registration.enabled и попробуйте зарегистрироваться) |
| `/webhooks` | Вебхуки: создайте эндпоинт, нажмите Send test, смотрите доставки |
| `/notifications` | Центр уведомлений; колокольчик в шапке с непрочитанными |
| `http://localhost:8025` | Mailpit: письма сброса пароля в dev-стеке (compose) |
| `http://localhost:8080` (или :5173 в dev-режиме) | SPA-логин (креды — плашка на странице) |
| Dashboard | KPI, график за 30 дней, live-лента событий (SSE) |
| Users | таблица с переключателем пагинации offset/cursor/keyset, поиск, CRUD |
| Jobs | очередь задач; статусы обновляются в реальном времени |
| Enrichment | постановка задач на обогащение (randomuser.me) и просмотр профилей |
| Reports | снапшот-отчёты + просмотр результата |
| Analytics | графики ClickHouse, трейл событий, live-фид |
| Profile | OAuth-идентичности, активные сессии, logout-all |
| Getting Started | встроенный тур по приложению |
| `http://localhost:8080/docs` | Scalar-справка по JSON API |
| `http://localhost:8080/health/ready` | готовность зависимостей |
| `http://localhost:8080/metrics` | Prometheus |
| Jaeger `:16686`, Grafana `:3000` | трейсинг и дашборды (`--profile observability`) |

Проверка через API:

```bash
curl -s -X POST localhost:8080/api/auth/login \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}' | jq -r .token
```

## Тесты и перфоманс

```bash
make test               # unit-тесты Go
make test-integration   # контрактные тесты хранилищ (testcontainers)
make test-e2e           # happy path против живого стека
make web-lint web-test  # oxlint/oxfmt и vitest для фронтенда
make seed-users USERS=20000   # bulk-сид perf-пользователей (пароль password123)
make perf-users MODE=keyset   # k6: p95 < 250ms на 100k пользователей
```
