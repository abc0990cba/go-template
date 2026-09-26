# RBAC: Casbin с доменами (организациями)

## Модель

Политики хранит Casbin с моделью RBAC + домены:

```
p = sub, dom, obj, act      # право: субъект, домен, объект, действие
g = _, _, _                 # назначение: пользователь, роль, домен
```

- Домен `*` — вся платформа; каждая организация — отдельный домен (её id).
- Правило действует, если субъект получил роль в домене запроса **или** в `*`, домены совпадают (`*` — wildcard), а объект/действие совпадают с учётом `keyMatch2`-wildcard (`users:*`, `*`).
- `p, superadmin, *, *, *` — полный доступ для роли superadmin.

Адаптер `internal/platform/rbac` сохраняет правила через порт `repository.RuleStore`
(таблица `casbin_rule`), так что in-memory режим и PostgreSQL взаимозаменяемы.

## Каталог прав

Ресурсы и действия объявлены в `internal/service/rbac.go` (`Resources`).
Новые защищённые маршруты:

1. добавьте ресурс/действие в каталог;
2. повесьте `app.requirePermission("resource", "action")` в `internal/httpapp/router.go`;
3. спрячьте UI-элементы через `useCan()(resource, action)`.

## Скоуп запроса

Middleware `orgDomain` выбирает домен каждого запроса: заголовок `X-Org-Id`
(требует членства или полного доступа), иначе единственная организация
пользователя, иначе `*`. UI держит выбранный скоуп в localStorage и шлёт
заголовок со всеми запросами.

## Системные роли

| Роль | Права (домен `*`) |
|------|-------------------|
| superadmin | всё, включая raw-редактор политик |
| admin | управление пользователями/ролями/org/настройками/вебхуками, аналитика, аудит |
| manager | чтение пользователей, jobs:execute, отчёты, аналитика |
| viewer | только чтение jobs/reports/enrichments/analytics |

Метаданные ролей — в таблице `roles` (`is_system` защищает от удаления).
Значения по умолчанию засеиваются при старте (`EnsureSystemRoles`).

## Bootstrap-права

Первый зарегистрированный аккаунт получает `superadmin` (в демо-данных это
плюс `demo-u-0001@demo.example` как admin), остальные — `viewer`.
Изменить поведение — `grantRegistrationRole` в `internal/httpapp/rbac.go`.

## Ключевые файлы

| Что | Где |
|-----|-----|
| Модель + адаптер + enforcer | `internal/platform/rbac/` |
| Сервис (роли, права, org) | `internal/service/rbac.go` |
| Middleware и хендлеры | `internal/httpapp/rbac.go` |
| Страницы UI | `web/src/routes/_auth.roles.tsx`, `_auth.policies.tsx`, `_auth.orgs.tsx` |
| Хук прав | `web/src/hooks/use-permissions.ts` |
| Контракт | `api/openapi/openapi.yaml` → тег `rbac` |
