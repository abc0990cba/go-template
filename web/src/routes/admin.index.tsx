import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { format } from "date-fns";
import {
  Activity,
  AtSign,
  Database,
  FileText,
  GitBranch,
  KeyRound,
  ListTodo,
  MailCheck,
  ThumbsUp,
  Users,
} from "lucide-react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { ExplainerBanner } from "@/components/explainer-banner";
import { KpiCard } from "@/components/kpi-card";
import { StatusBadge } from "@/components/status-badge";
import { useLive } from "@/hooks/use-event-stream";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { api } from "@/lib/api";

export const Route = createFileRoute("/admin/")({
  staticData: { title: "Dashboard" },
  component: DashboardPage,
});

const summaryQuery = {
  queryKey: ["analytics", "summary"],
  queryFn: api.analyticsSummary,
};
const timeseriesQuery = {
  queryKey: ["analytics", "timeseries"],
  queryFn: () => api.analyticsTimeseries(30),
};
const recentJobsQuery = {
  queryKey: ["jobs", "recent"],
  queryFn: () => api.listJobs({ mode: "offset", limit: 6 }),
};

const chartConfig = {
  user_registered: { label: "Registrations", color: "var(--color-chart-1)" },
  user_logged_in: { label: "Logins", color: "var(--color-chart-2)" },
  job_done: { label: "Jobs done", color: "var(--color-chart-3)" },
} satisfies ChartConfig;

function DashboardPage() {
  const { data: summary } = useSuspenseQuery(summaryQuery);
  const { data: series } = useSuspenseQuery(timeseriesQuery);
  const { data: jobs } = useSuspenseQuery(recentJobsQuery);
  const { feed } = useLive();

  // The API nests per-event counters under events:{}; recharts reads flat
  // keys, so lift them up — Line dataKey="user_registered" expects a
  // top-level field.
  const chartData = (series ?? []).map((point) => ({ ...point, ...point.events }));

  return (
    <div className="space-y-4">
      <PageIntro />

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard
          title="Total users"
          value={summary.total_users}
          icon={<Users className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="Password identities"
          value={summary.password_identities}
          icon={<KeyRound className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="Google identities"
          value={summary.google_identities}
          icon={<MailCheck className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="GitHub identities"
          value={summary.github_identities}
          icon={<GitBranch className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="Facebook identities"
          value={summary.facebook_identities}
          icon={<ThumbsUp className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="Twitter identities"
          value={summary.twitter_identities}
          icon={<AtSign className="size-4 text-muted-foreground" />}
        />
        <KpiCard
          title="Jobs done"
          value={summary.jobs_done}
          icon={<ListTodo className="size-4 text-muted-foreground" />}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">Events over the last 30 days</CardTitle>
            <CardDescription>
              Aggregated from the ClickHouse trail (or the in-memory trail in dev mode). Updates
              arrive over SSE.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ChartContainer config={chartConfig} className="h-64 w-full">
              {/* recharts crashes on null data; the API degrades to [] but stay defensive */}
              <LineChart data={chartData} margin={{ left: -20, right: 12 }}>
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey="day"
                  tickLine={false}
                  axisLine={false}
                  tickMargin={8}
                  minTickGap={40}
                />
                <YAxis tickLine={false} axisLine={false} allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent indicator="line" />} />
                <Line
                  dataKey="user_registered"
                  type="monotone"
                  stroke="var(--color-user_registered)"
                  strokeWidth={2}
                  dot={false}
                />
                <Line
                  dataKey="user_logged_in"
                  type="monotone"
                  stroke="var(--color-user_logged_in)"
                  strokeWidth={2}
                  dot={false}
                />
                <Line
                  dataKey="job_done"
                  type="monotone"
                  stroke="var(--color-job_done)"
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ChartContainer>
          </CardContent>
        </Card>

        <LiveFeed feed={feed} />
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">Recent jobs</CardTitle>
            <CardDescription>
              Newest background jobs; statuses update live while you watch.
            </CardDescription>
          </div>
          <Link to="/admin/jobs" className="text-sm font-medium underline underline-offset-4">
            All jobs
          </Link>
        </CardHeader>
        <CardContent>
          <ul className="divide-y text-sm">
            {(jobs.data ?? []).map((job) => (
              <li key={job.id} className="flex items-center justify-between gap-3 py-2">
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs">{job.id}</p>
                  <p className="text-xs text-muted-foreground">
                    {job.type} · {format(new Date(job.created_at), "MMM d, HH:mm:ss")}
                  </p>
                </div>
                <StatusBadge status={job.status} />
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}

function PageIntro() {
  return (
    <>
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          One screen to answer: are users signing up, is the queue moving, is the warehouse
          receiving events.
        </p>
      </div>
      <ExplainerBanner
        title="What am I looking at?"
        howTo={[
          "The tiles above are operational counters from PostgreSQL — they never depend on the warehouse being up.",
          "The chart reads daily aggregates from ClickHouse; the seed backfilled 30 days of events so it has history.",
          "The Live panel is fed by a Server-Sent Events stream — open /users in another tab and delete someone to watch it react.",
        ]}
      >
        This console fronts a Go template backend: a JSON API, a job queue, and a ClickHouse event
        warehouse. Everything you see here exists as an endpoint under <code>/api</code> — the{" "}
        <a href="/docs" className="underline">
          API docs
        </a>{" "}
        render the same contract the frontend is built on.
      </ExplainerBanner>
    </>
  );
}

function LiveFeed({ feed }: { feed: ReturnType<typeof useLive>["feed"] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Activity className="size-4" /> Live event trail
        </CardTitle>
        <CardDescription>
          <Database className="mr-1 inline size-3.5" />
          Pushed over Server-Sent Events as it happens.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {feed.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Nothing yet. Register a user or run a job — events appear here the moment they are
            emitted.
          </p>
        ) : (
          <ul className="max-h-64 space-y-1.5 overflow-y-auto text-sm">
            {feed.map((event, index) => (
              <li key={`${event.occurred_at}-${index}`} className="flex items-center gap-2">
                <FileText className="size-3.5 text-muted-foreground" />
                <span className="font-mono text-xs">{event.name}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {format(new Date(event.occurred_at), "HH:mm:ss")}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
