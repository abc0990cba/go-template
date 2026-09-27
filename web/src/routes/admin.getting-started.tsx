import { createFileRoute } from "@tanstack/react-router";
import { BookOpen, Database, GitBranch, ListTodo, Rocket, Server, Users } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

export const Route = createFileRoute("/admin/getting-started")({
  staticData: { title: "Getting Started" },
  component: GettingStartedPage,
});

function GettingStartedPage() {
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Getting Started</h1>
        <p className="text-sm text-muted-foreground">
          The in-app tour of this template: what each piece is, where to poke it, and what to read
          next.
        </p>
      </div>

      <Alert>
        <Rocket className="size-4" />
        <AlertTitle>You are signed in against the live stack</AlertTitle>
        <AlertDescription>
          Everything below works on the running instance — no separate sandbox needed. Open pages in
          two tabs to watch the live updates cross over.
        </AlertDescription>
      </Alert>

      <ExplainerBanner
        title="Five minutes around the app"
        defaultOpen
        howTo={[
          "Users — create, edit, delete, and switch pagination modes; search is server-side.",
          "Enrichment — queue external profile fetching and watch the job lifecycle on Jobs with live SSE updates.",
          "Reports — generate a snapshot, then open it in the side sheet.",
          "Analytics — charts from the event warehouse plus the raw trail and the live feed.",
          "Profile — link Google/GitHub (if configured), inspect sessions, log out everywhere.",
        ]}
      >
        The app is a React SPA (Vite + TanStack Router/Query/Table, shadcn/ui) talking to a Go JSON
        API. In production the built SPA is embedded in the Go binary; in development Vite proxies{" "}
        <code>/api</code> to the server.
      </ExplainerBanner>

      <div className="grid gap-4 md:grid-cols-2">
        <FeatureCard
          icon={<Server className="size-4" />}
          title="The Go backend"
          body="Gin router, sqlc-generated queries, golang-migrate migrations, asynq job queue, ClickHouse warehouse, Prometheus metrics, OpenTelemetry traces. The JSON contract lives in api/openapi/openapi.yaml and CI rejects drift between the spec and the generated types."
          link="/docs"
          linkLabel="Scalar API reference"
        />
        <FeatureCard
          icon={<ListTodo className="size-4" />}
          title="Jobs two ways"
          body="With APP_REDIS_ADDR set, jobs run in a separate worker via asynq (with a nightly report cron). Without Redis, jobs queue in the database and Process one job runs them in-process — ideal for demos."
        />
        <FeatureCard
          icon={<Database className="size-4" />}
          title="Analytics warehouse"
          body="Named events buffer in memory and flush to ClickHouse in batches. Dashboards never depend on the warehouse: operational counters come from PostgreSQL and degrade gracefully if the warehouse is down."
        />
        <FeatureCard
          icon={<Users className="size-4" />}
          title="Pagination as a feature"
          body="Every collection supports offset, cursor, and keyset modes behind one query-string vocabulary. The k6 scripts (make perf-users MODE=keyset) assert latency targets per mode on 100k rows."
        />
        <FeatureCard
          icon={<GitBranch className="size-4" />}
          title="OAuth linking"
          body="Google and GitHub attach as extra identities on an existing account (link-only by design; the flows need client credentials configured to enable the buttons on Profile)."
        />
        <FeatureCard
          icon={<BookOpen className="size-4" />}
          title="Reading material"
          body="docs/quickstart.md — run the full stack; docs/frontend.md — the SPA stack and conventions; docs/analytics.md — the event pipeline; docs/performance.md — targets and how to measure."
        />
      </div>
    </div>
  );
}

function FeatureCard({
  icon,
  title,
  body,
  link,
  linkLabel,
}: {
  icon: React.ReactNode;
  title: string;
  body: string;
  link?: string;
  linkLabel?: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-base">
          {icon}
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <CardDescription>{body}</CardDescription>
        {link ? (
          <a
            href={link}
            target="_blank"
            rel="noreferrer"
            className="mt-2 inline-block text-sm font-medium underline underline-offset-4"
          >
            {linkLabel}
          </a>
        ) : null}
      </CardContent>
    </Card>
  );
}
