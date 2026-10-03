import type { Dictionary } from "./en";

/** Deutsche Version. Technische Begriffe (RBAC, TOTP, Outbox, …) bleiben
 * englisch, wie es in der deutschsprachigen Fachdokumentation üblich ist. */
export const de: Dictionary = {
  meta: {
    title: "Go Template — Die Go- und React-Plattform, die als ein Binary ausgeliefert wird",
  },
  nav: {
    features: "Funktionen",
    how: "So funktioniert es",
    faq: "FAQ",
    signIn: "Anmelden",
    getStarted: "Loslegen",
  },
  lang: {
    label: "Sprache",
  },
  hero: {
    badge: "Open-Source-Starterplattform",
    titlePre: "Die Go- und React-Plattform, die als",
    titleAccent: "ein Binary",
    titlePost: "ausgeliefert wird",
    subtitle:
      "Authentifizierung, RBAC, Tickets mit Freigaben, Hintergrundaufgaben, Event-Streams und Analytik — ein produktionsreifes Fundament, das Sie klonen, erweitern und in wenigen Minuten ausrollen können.",
    ctaPrimary: "Kostenloses Konto erstellen",
    ctaSecondary: "Demo ansehen",
    checks: [
      "Ein Container, ein Port",
      "Kein CORS, kein separates Hosting",
      "Health-Probes inklusive",
    ],
  },
  stats: [
    { value: "1", label: "Binary zum Ausrollen — Frontend eingebettet" },
    { value: "18", label: "fertige Funktionsbereiche" },
    { value: "26", label: "dokumentierte Architekturentscheidungen" },
    { value: "50+", label: "Seiten Dokumentation" },
  ],
  features: {
    title: "Alle Funktionen eines echten Produkts",
    subtitle:
      "Achtzehn fertige Funktionsbereiche, kein Hello-World. Klicken Sie auf eine Karte, um zu sehen, was drinsteckt — die Verdrahtung ist erledigt, Ihr erster Commit ist ein Feature.",
    more: "Details ansehen",
    dialogClose: "Schließen",
    whatsInside: "Das ist drin",
    items: {
      auth: {
        title: "Authentifizierung & MFA",
        teaser:
          "Cookie-Sessions, OAuth-Identitäten, TOTP-Zweifaktor mit Wiederherstellungscodes, E-Mail-Verifizierung — komplett von A bis Z.",
        description:
          "Alles, was ein echter Login-Flow braucht, am ersten Tag fertig: Cookie-Sessions mit gleitender Ablaufzeit, OAuth-Identitätsverknüpfung, TOTP-Registrierung per QR-Code, einmalig verwendbare, gehashte Wiederherstellungscodes und ein mfa.required-Policy-Flag, das 2FA für einen Nutzer oder eine ganze Organisation erzwingt.",
        bullets: [
          "Cookie-Sessions mit Rotation und Widerruf",
          "TOTP-Registrierung per QR-Code mit Wiederherstellungscodes",
          "E-Mail-Verifizierung und Passwort-Reset mit Einmal-Tokens",
          "OAuth-Identitätsverknüpfung und erzwungene MFA-Policy",
        ],
      },
      rbac: {
        title: "RBAC & Policies",
        teaser:
          "Casbin-Rollen mit Vererbung, Ressource:Aktion-Berechtigungsmatrix und Live-Policy-Editor.",
        description:
          "Rollenbasierte Zugriffskontrolle mit Casbin: Rollenvererbung, Multi-Tenancy über Domänen und eine Ressource:Aktion-Matrix, die Sie live in der Admin-Konsole bearbeiten. Jede Anfrage wird serverseitig neu geprüft, und jede Policy-Änderung landet im Audit-Log.",
        bullets: [
          "Rollen mit Vererbung und Grants pro Organisation",
          "Ressource:Aktion-Matrix mit Raw-Policy-Editor",
          "Admin-Impersonation mit sichtbarem Banner und Audit-Trail",
          "Berechtigungsprüfungen gecacht, bei Policy-Änderungen invalidiert",
        ],
      },
      orgs: {
        title: "Organisationen & Einladungen",
        teaser:
          "Echte Tenants mit Mitgliedern, org-spezifischem API-Zugriff und Einladungen, die in einem Schritt angenommen werden.",
        description:
          "Casbin-Domänen machen Organisationen zu echten Tenants: Mitglieder, tenant-spezifische Rollen und org-spezifischer API-Zugriff über den X-Org-Id-Header. E-Mail-Einladungen tragen Einmal-Tokens und verleihen Mitgliedschaft und Domänenrolle in einem einzigen Schritt.",
        bullets: [
          "Multi-Tenancy über Casbin-Domänen mit Scope-Umschalter",
          "Organisationsmitglieder mit Rollen pro Tenant",
          "E-Mail-Einladungen mit Einmal-Tokens",
          "Annahme in einem Schritt: Mitgliedschaft und Rolle in einer Transaktion",
        ],
      },
      tickets: {
        title: "Tickets & Freigaben",
        teaser:
          "Ein Jira-artiger Antrags-Workflow: Zustandsautomat, mehrstufige Freigabeketten, Ansichten für Antragsteller und Freigeber.",
        description:
          "Ein Workflow in Produktionsform: ein Ticket-Zustandsautomat mit unterschiedlichen Ausgängen „vom Antragsteller zurückgezogen“ und „vom Freigeber abgelehnt“, konfigurierbare mehrstufige Freigabeketten pro Tickettyp und getrennte Bereiche „Meine Tickets“ und „Freigaben“.",
        bullets: [
          "Zustandsautomat mit Nacharbeit und Ergebnisübergabe",
          "Konfigurierbare mehrstufige Freigabeketten pro Tickettyp",
          "Rollengebundene Genehmigungs- und Ablehnungsentscheidungen",
          "Getrennte Oberflächen für Antragsteller und Freigeber",
        ],
      },
      audit: {
        title: "Audit-Log",
        teaser:
          "Unveränderliche Wer/Was/Wann-Einträge mit Vorher/Nachher-Diffs, IP und Request-ID.",
        description:
          "Jede bedeutsame Änderung schreibt einen unveränderlichen Audit-Eintrag: Akteur, Aktion, Vorher/Nachher-Diff, IP-Adresse und Request-ID — in derselben Transaktion wie die Änderung, mit filterbarer Admin-Ansicht obendrauf.",
        bullets: [
          "Vorher/Nachher-JSON-Diffs für jede Änderung",
          "Akteur-, IP- und Request-ID-Korrelation",
          "Filterbare Ansicht in der Admin-Konsole",
          "Schreiben in derselben Transaktion — keine verpassten Einträge",
        ],
      },
      settings: {
        title: "Einstellungen & Feature-Flags",
        teaser:
          "Typisierte Key-Value-Einstellungen in der Datenbank, Einstellungs-UI, öffentliche Flags und Read-Through-Caching.",
        description:
          "Ein typisierter Einstellungsspeicher in PostgreSQL mit einer UI zum Ändern: Validierung pro Schlüssel, öffentliche Flags, die vor dem Login lesbar sind, und ein Read-Through-Cache, damit heiße Einstellungen nie bei jeder Anfrage die Datenbank berühren.",
        bullets: [
          "Typisierte Schlüssel mit Validierung",
          "Öffentliche Flags für Seiten vor dem Login",
          "Einstellungs-UI in der Admin-Konsole",
          "Read-Through-Caching mit Invalidierung",
        ],
      },
      jobs: {
        title: "Hintergrundaufgaben",
        teaser: "Redis-Warteschlange (asynq) mit Retries, Scheduling und einer Live-Konsole.",
        description:
          "Ein echter Worker-Prozess auf asynq und Redis: idempotentes Einreihen, Retries mit Backoff, verzögerte und periodische Aufgaben, Dead-Letter-Behandlung und eine Jobs-Seite mit Live-Status über Server-Sent Events.",
        bullets: [
          "Separater Worker-Prozess mit idempotentem Einreihen",
          "Retries mit Backoff, verzögerte und periodische Aufgaben",
          "Live-Warteschlangenkonsole mit SSE-Updates",
          "Graceful Shutdown mit Kontext-Weiterreichung",
        ],
      },
      streams: {
        title: "Event-Streams & Outbox",
        teaser:
          "Kafka- und NATS-JetStream-Konsumenten mit Exactly-once-Verarbeitung und DLQ-Replay-Konsole.",
        description:
          "Nutzeranreicherung aus zwei externen Systemen über Kafka und NATS JetStream: transaktionales Outbox-Relay, Inbox-Dedupe, Sequenz-Guards pro Quelle und zeilengesperrte Feld-Ownership — exactly once, mit Dead-Letter-Queue und Admin-Replay-Konsole.",
        bullets: [
          "Transaktionales Outbox → Relay → Broker",
          "Inbox-Dedupe und Sequenz-Guards (exactly once)",
          "Dead-Letter-Queue mit Admin-Replay",
          "Kafka- und NATS-JetStream-Adapter",
        ],
      },
      webhooks: {
        title: "Webhooks",
        teaser:
          "Event-Abos mit HMAC-SHA256-Signaturen, Warteschlangen-Retries und Zustellungsprotokoll.",
        description:
          "Domänen-Events abonnieren und beliebig zustellen: HMAC-SHA256-signierte Payloads, Zustellung über die Warteschlange mit Retries und Backoff, ein vollständiges Zustellungsprotokoll mit Antwortcodes und eine Testsenden-Schaltfläche pro Abo.",
        bullets: [
          "HMAC-SHA256-Signatur bei jeder Zustellung",
          "Retries über die Warteschlange mit Backoff",
          "Zustellungsprotokoll mit Antwortcodes",
          "Testsendung mit einem Klick",
        ],
      },
      notifications: {
        title: "Benachrichtigungen",
        teaser: "In-App-Benachrichtigungscenter mit Ungelesen-Badge und Live-Toasts per SSE.",
        description:
          "Ein Benachrichtigungscenter in der App: Ungelesen-Badge, Als-gelesen-Markierungen und Live-Toasts über Server-Sent Events — ohne Polling und ohne Infrastruktur über das Binary hinaus.",
        bullets: [
          "Benachrichtigungscenter mit Ungelesen-Badge",
          "Live-Toasts über Server-Sent Events",
          "Als gelesen und alles markieren",
          "Benachrichtigungseinstellungen pro Nutzer",
        ],
      },
      apikeys: {
        title: "API-Keys & JWT-Grants",
        teaser:
          "gtpl_-Bearer-Tokens mit Scope-Einschränkung, dazu OAuth-artige JWT-Grants mit Rotation.",
        description:
          "Zwei erstklassige Token-Systeme: persönliche API-Keys (gtpl_…), die einmal angezeigt, gehasht gespeichert und widerrufbar sind, mit Scope-Einschränkung auf Objekt:Aktion — und OAuth-artige Passwort-, MFA- und Refresh-Grants mit Rotation und Wiederverwendungserkennung für mobile und API-Clients.",
        bullets: [
          "API-Keys: einmal angezeigt, gehasht gespeichert, widerrufbar",
          "Scope-Einschränkung (Objekt:Aktion) pro Key",
          "JWT-Refresh-Rotation mit Wiederverwendungserkennung",
          "Passwort-, MFA- und Refresh-Grants",
        ],
      },
      files: {
        title: "Dateispeicher",
        teaser:
          "S3/MinIO-Blobs mit Prüfsummen, berechtigungsgeprüften Downloads und Presigned URLs.",
        description:
          "Uploads mit Prüfsummen zu S3 oder MinIO (In-Memory für die Entwicklung), berechtigungsgeprüfte Downloads, Ticket-Anhänge und Presigned URLs — die Aufbewahrungsfristen räumt der Worker auf.",
        bullets: [
          "S3-, MinIO- und In-Memory-Adapter",
          "Prüfsummenprüfung jedes Blobs",
          "Berechtigungsgeprüfte Downloads",
          "Presigned URLs für direkten Zugriff",
        ],
      },
      export: {
        title: "DSGVO-Datenexport",
        teaser:
          "Ein Job in der Warteschlange baut ein ZIP mit JSON pro Entität, Download nur für Eigentümer, 7 Tage Aufbewahrung.",
        description:
          "Ein DSGVO-geformter Datenexport: im Profil anfordern, ein Job in der Warteschlange baut ein ZIP mit JSON pro Entität (Profil, Tickets, Audit, Dateien), Download-Links gehören nur dem Eigentümer und laufen ab, alte Archive werden automatisch gelöscht.",
        bullets: [
          "Anforderung mit einem Klick von der Profilseite",
          "ZIP mit JSON pro Entität, gebaut vom Worker",
          "Nur für den Eigentümer bestimmte, ablaufende Links",
          "Automatische Aufräumung nach 7 Tagen",
        ],
      },
      search: {
        title: "Caching & Suche",
        teaser:
          "Read-Through-Cache mit Singleflight, ETag/304 auf schweren Listen und berechtigungsgeprüfte Volltextsuche.",
        description:
          "Heiße Lesezugriffe kommen aus einem Memory- oder Redis-Cache mit Singleflight, damit ein Fehltreffer auf einem heißen Schlüssel kein Stauflug auf den Speicher wird; schwere Listen antworten per ETag mit 304, und die Volltextsuche über Nutzer und Tickets ist berechtigungsgeprüft und gerankt.",
        bullets: [
          "Read-Through-Cache (Memory/Redis) mit Singleflight",
          "ETag-/If-None-Match-Revalidierung mit 304",
          "Postgres-tsvector-Suche mit Ranking",
          "Ergebnisse im Rahmen der Rechte des Aufrufers",
        ],
      },
      resilience: {
        title: "Resilienz-Werkzeugkasten",
        teaser:
          "Circuit Breaker, verteiltes Rate Limiting, Idempotency-Keys und Bulkheads — alles mit Metriken.",
        description:
          "Die Ausfallarten sind behandelt, bevor Sie ausliefern: Circuit Breaker um jeden ausgehenden Aufruf, verteiltes Rate Limiting auf Redis mit In-Memory-Fallback, Idempotency-Key-Middleware, die Client-Wiederholungen in Antwort-Replays verwandelt, und Bulkheads pro Abhängigkeit — alles als Prometheus-Metriken exportiert.",
        bullets: [
          "Circuit Breaker mit Zustandsmetriken",
          "Verteiltes Rate Limiting mit In-Memory-Fallback",
          "Idempotency-Key-Replay-Middleware",
          "Bulkheads mit begrenzter Nebenläufigkeit",
        ],
      },
      observability: {
        title: "Observability & SLOs",
        teaser: "OpenTelemetry-Traces, RED-Metriken, SLO-Burn-Rate-Alerts und Dashboards als Code.",
        description:
          "Logs, Metriken und Traces mit Trace↔Log-Korrelation an jeder Grenze — HTTP, Jobs, Kafka/NATS, pgx, Redis, ClickHouse — plus Prometheus-RED-Metriken, Continuous Profiling sowie Dashboards, Alert-Regeln und SLO-Burn-Rate-Alerts als Code ausgeliefert.",
        bullets: [
          "OpenTelemetry-Traces über jede Grenze",
          "Prometheus-RED-Metriken und Pool-Statistiken",
          "Dashboards und Alert-Regeln als Code",
          "SLO-Burn-Rate-Alerting und Continuous Profiling",
        ],
      },
      deploy: {
        title: "Überall ausrollen",
        teaser: "Ein Binary — oder Helm mit SLO-gegatetem Canary, GitOps-Manifeste oder Nomad.",
        description:
          "Liefern Sie, wie Sie wollen: ein einziges Distroless-Binary mit eingebettetem Frontend; ein Helm-Chart mit HPA, PDB, Probes und Secret-Mounts; ein SLO-gegateter Canary via Argo Rollouts mit Auto-Rollback; GitOps-Manifeste; oder ein Nomad-Job. Releases sind signiert, gescannt und SBOM-belegt.",
        bullets: [
          "Ein statisches Binary, Frontend inklusive",
          "Helm-Chart mit SLO-gegatetem Canary",
          "GitOps-Manifeste und eine Nomad-Alternative",
          "Signierte, gescannte, SBOM-belegte Releases",
        ],
      },
      dx: {
        title: "Developer Experience",
        teaser:
          "Demos mit einem Befehl, ein reicher Seed-Datensatz, ein Regressionsspaziergang und OpenAPI-Codegen.",
        description:
          "Das Template ist zum Erkunden gebaut: make demo-up startet die Broker, devtools veröffentlicht echte Events, der Seed-Datensatz füllt beim ersten Start jeden Bildschirm, ein Regressionsskript fährt jedes Feature Ende-zu-Ende, und der API-Client des Frontends wird aus der OpenAPI-Spezifikation generiert.",
        bullets: [
          "make demo-*-Szenarien: Anreicherung, DLQ, Breaker…",
          "Reicher Demo-Datensatz beim ersten Start",
          "Regressionsspaziergang über jedes Feature",
          "OpenAPI → TypeScript-Client-Codegen",
        ],
      },
    },
  },
  how: {
    title: "Vom Klon zur Produktion in drei Schritten",
    subtitle: "Keine Verdrahtungswochen. Die langweiligen Teile sind fertig und getestet.",
    steps: [
      {
        title: "Klonen und konfigurieren",
        text: "Umgebungsgesteuerte Konfiguration, docker compose für Postgres & Freunde und ein gesäter Demo-Datensatz zum Erkunden ab dem ersten Start.",
      },
      {
        title: "Domäne erweitern",
        text: "Tickettypen, Rollen und Handler ergänzen — dateibasierte Routen, der generierte Routenbaum und die OpenAPI-Spezifikation halten automatisch mit.",
      },
      {
        title: "Ausliefern",
        text: "docker compose, Helm oder Nomad. Health-Probes, Metriken, Traces und Rate Limiting sind inklusive.",
      },
    ],
  },
  stack: {
    title: "Ein absichtlich langweiliger, bewährter Stack",
    subtitle:
      "Technologien, die Ihr Team schon kennt — gewählt für die nächsten fünf Jahre, nicht für den nächsten Hype.",
  },
  faq: {
    title: "Häufige Fragen",
    items: [
      {
        q: "Was ist Go Template?",
        a: "Eine Starterplattform in Produktionsform: eine Go-API und eine React-Admin-Konsole mit Authentifizierung, RBAC, Tickets, Hintergrundaufgaben, Live-Streams und Analytik — alles, was ein echtes internes Produkt am ersten Tag braucht.",
      },
      {
        q: "Brauche ich Kubernetes, um es zu betreiben?",
        a: "Nein. Ein statisches Go-Binary plus Postgres genügt; das Frontend ist im Binary selbst eingebettet. Docker-Compose-Dateien, ein Helm-Chart und ein Nomad-Job liegen bei, sobald ein einzelner Host nicht mehr reicht.",
      },
      {
        q: "Wie wird das Web-Frontend ausgeliefert?",
        a: "Die React-App baut zu statischen Assets, die in das Go-Binary eingebettet und same-origin ausgeliefert werden — kein CORS, kein separates Hosting, und die Landing Page kommt als vorgerendertes HTML für Crawler.",
      },
      {
        q: "In welchen Sprachen gibt es die Oberfläche?",
        a: "Diese Seite und die Anwendungsoberfläche gibt es auf Englisch, Russisch, Französisch und Deutsch — in der Kopfzeile wählen, die Wahl wird gemerkt.",
      },
      {
        q: "Kann ich es als Basis für ein echtes Produkt nutzen?",
        a: "Genau dafür ist es da. Der Stack ist absichtlich langweilig — Go, Postgres, React, Tailwind — und die Verdrahtung (Sessions, RBAC, Audit, Jobs, Observability) ist erledigt, damit Sie mit den Funktionen starten, die Ihr Produkt besonders machen.",
      },
      {
        q: "Ist es kostenlos?",
        a: "Ja — klonen, erweitern, ausliefern. Keine Bezahlstufen, keine Lizenzschlüssel, kein Nach-Hause-Telefonieren.",
      },
    ],
  },
  cta: {
    title: "Bereit für einen Blick nach innen?",
    text: "Erstellen Sie ein Konto und durchlaufen Sie den gesamten Ablauf — vom Ticket bis zur Freigabekette — mit den gesäten Demo-Daten.",
    button: "Kostenlos starten",
  },
  footer: {
    product: "Produkt",
    app: "Anwendung",
    apiDocs: "API-Dokumentation",
    openapiSpec: "OpenAPI-Spezifikation",
    createAccount: "Konto erstellen",
    rights: "© 2026 Go Template",
  },
};
