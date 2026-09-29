# 02 — System Context (C4 Level 1)

**In one sentence:** one product — *the Go Template application* — serves
browser users and programmatic clients through a single HTTP entry point,
and integrates with six kinds of external systems: identity providers,
email, event brokers, object storage, an analytics warehouse, and the
Kubernetes/compose platform that runs it.

The C4 **System Context** diagram is the highest-level view: people and
systems that interact with our software, nothing about internals.

## 2.1 Actors

| Actor | Interacts via | Typical intent |
|---|---|---|
| **End user** (requester / approver) | SPA at `/` (cookie session) | File tickets, approve requests, read notifications, manage own profile, MFA, API keys |
| **Admin / manager** | SPA at `/admin/*` (cookie session) | Manage users, roles, orgs, settings/flags, webhooks, streams, audit; approve first chain steps |
| **API / mobile client** | `Authorization: Bearer` (JWT grant or `gtpl_` API key) | Integrate ticket and user data programmatically |
| **Ops engineer** | `/metrics`, `/health/*`, `/version`, Grafana/Prometheus/Jaeger | Monitor, deploy, toggle maintenance mode |
| **Webhook consumer** | Receives signed HTTP POSTs | React to product events (e.g. ticket approved) |
| **Developer** | `make *`, compose profiles, `cmd/devtools`, Scalar `/docs` | Run, test, demonstrate |

## 2.2 Context diagram

```mermaid
flowchart TB
    classDef person fill:#e8f0fe,stroke:#4a6fa5,color:#1a2b4a
    classDef system fill:#dcecdc,stroke:#3a7a3a,color:#143314
    classDef ext fill:#f4f4f4,stroke:#888,color:#333

    subgraph people["People"]
        U["End user<br/>(client app)"]
        A["Admin / manager<br/>(admin console)"]
        D["Developer"]
    end

    subgraph ours["Go Template application"]
        SYS["Full-stack app<br/>Go API + Worker + React SPA<br/>serves http://localhost:8080"]
    end

    subgraph external["External systems"]
        OIDC["OAuth providers<br/>Google / GitHub"]
        MAIL["SMTP server<br/>(Mailpit in dev)"]
        KAFKA["Kafka broker<br/>(Redpanda in dev)"]
        NATS["NATS JetStream"]
        S3["S3-compatible object store<br/>(MinIO in dev)"]
        CH["ClickHouse<br/>analytics warehouse"]
        WH["Webhook consumers<br/>(customer systems)"]
        PLAT["Kubernetes / Docker<br/>Helm chart, GHCR images"]
    end

    U -->|HTTPS: browser session| SYS
    A -->|HTTPS: browser session| SYS
    D -->|make, devtools, k6| SYS
    SYS -->|"OAuth 2.0 authorization code (link identity)"| OIDC
    SYS -->|SMTP: reset / verify / invitation mail| MAIL
    SYS -->|"consume: user-profile.v1 · publish: user-events.v1"| KAFKA
    SYS -->|"consume: users.contact.v1 · publish: users.events.v1"| NATS
    SYS -->|S3 API: put/get/presign blobs| S3
    SYS -->|"events insert + analytics queries"| CH
    SYS -->|"HMAC-SHA256 signed POST deliveries"| WH
    SYS -->|"signed multi-arch image + Helm chart"| PLAT
```

## 2.3 Trust boundaries

1. **Browser ↔ API** — the only untrusted input surface. Protected by
   session cookies (`SameSite=Lax`, `HttpOnly`, `Secure`), an Origin/`Sec-Fetch-Site`
   CSRF check on non-GET API calls, body size caps, per-IP and per-identity
   rate limits, and security headers.
2. **API ↔ external systems** — every outbound call is breaker-wrapped and
   time-boxed (`APP_EXTERNAL_TIMEOUT`, default 3 s); webhook payloads are
   HMAC-signed so *consumers* can trust *us*.
3. **Broker ↔ worker** — consumers authenticate to the broker; messages are
   validated and malformed payloads are parked in a dead-letter queue
   rather than crash-looping the consumer.

## 2.4 What the system does *not* talk to

- No third-party analytics/tracking services; events go to the
  self-hosted ClickHouse.
- No external secrets manager — secrets come from the environment or
  mounted files (`*_FILE` convention), supplied by the platform
  (Kubernetes secrets, compose env).
- No CDN — the SPA ships inside the binary; assets are same-origin.

Next: [03 — Containers & Runtime Views](03-containers-and-runtime-views.md)
— what actually runs, process by process.
