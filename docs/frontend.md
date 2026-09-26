# Frontend

The frontend is a server-rendered app built from templ components (`internal/httpapp/views.templ`), with HTMX available for progressive enhancement. Regenerate after edits with `make generate`; the generated `views_templ.go` is committed so CI can verify it does not drift.

Pages:

- `/register`
- `/login`
- `/`
- `/profile`
- `/users`
- `/enrichment`
- `/jobs`
- `/reports`
- `/analytics`
- `/docs`

The UI is a working dashboard, not a landing page. Every core backend capability should be testable from the browser.

htmx partial updates (users list, live job status) remain the next natural UI step.
