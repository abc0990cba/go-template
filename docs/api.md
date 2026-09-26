# API

OpenAPI is the source of truth for HTTP behavior.

Files:

- `api/openapi/openapi.yaml`
- `/openapi.yaml`
- `/docs`

Rules:

- Use nouns for resources.
- Use standard HTTP methods.
- Return `application/problem+json` style error bodies.
- Document pagination parameters.
- Version breaking changes.
