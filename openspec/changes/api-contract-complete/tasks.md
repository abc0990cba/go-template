## 1. Contract

- [x] 1.1 Rewrite openapi.yaml v1.0.0: all operations, response schemas, problem schema, security
- [x] 1.2 Add missing endpoints: users get/put/delete, jobs list/enqueue, reports list/enqueue
- [x] 1.3 Generated wire types (oapi-codegen models); handlers bind against them

## 2. Enforcement And Docs

- [x] 2.1 make generate runs sqlc + oapi-codegen; CI generated-code job rejects drift
- [x] 2.2 Endpoint tests (user CRUD by id, jobs, reports, 404 problem)
- [x] 2.3 docs/api.md rewritten
