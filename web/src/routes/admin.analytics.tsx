import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { format } from "date-fns";
import { useEffect, useState } from "react";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ExplainerBanner } from "@/components/explainer-banner";
import { KpiCard } from "@/components/kpi-card";
import { useLive } from "@/features/live";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import { queryClient } from "@/shared/lib/query-client";

export const Route = createFileRoute("/admin/analytics")({
  staticData: { title: "Analytics" },
  component: AnalyticsPage,
});

const EVENT_NAMES = [
  "user_registered",
  "user_logged_in",
  "oauth_linked",
  "job_queued",
  "job_done",
  "job_failed",
] as const;

const chartConfig = {
  total: { label: "All events", color: "var(--color-chart-1)" },
  count: { label: "Count", color: "var(--color-chart-1)" },
  registered: { label: "DAU", color: "var(--color-chart-1)" },
  user_registered: { label: "Registrations", color: "var(--color-chart-1)" },
  user_logged_in: { label: "Logins", color: "var(--color-chart-2)" },
  oauth_linked: { label: "OAuth links", color: "var(--color-chart-3)" },
  job_queued: { label: "Queued", color: "var(--color-chart-4)" },
  job_done: { label: "Done", color: "var(--color-chart-2)" },
  job_failed: { label: "Failed", color: "var(--color-chart-5)" },
} satisfies ChartConfig;

function AnalyticsPage() {
  const [days, setDays] = useState("30");
  const [view, setView] = useState<EventView>("total");
  const { data: summary } = useSuspenseQuery(summaryQuery);
  const { feed } = useLive();
  const live = useLive();

  const series = useQuery({
    queryKey: ["analytics", "timeseries", days],
    queryFn: () => api.analyticsTimeseries(Number.parseInt(days, 10)),
    placeholderData: (previous) => previous,
  });
  const recent = useQuery({
    queryKey: ["analytics", "recent-events"],
    queryFn: () => api.recentEvents(50),
    placeholderData: (previous) => previous,
  });
  const dau = useQuery({
    queryKey: ["analytics", "dau", days],
    queryFn: () => api.analyticsDAU(Number.parseInt(days, 10)),
    placeholderData: (previous) => previous,
  });
  const featureBreakdown = useQuery({
    queryKey: ["analytics", "breakdown", "feature", days],
    queryFn: () => api.analyticsBreakdown("feature", Number.parseInt(days, 10)),
    placeholderData: (previous) => previous,
  });
  const countryBreakdown = useQuery({
    queryKey: ["analytics", "breakdown", "country", days],
    queryFn: () => api.analyticsBreakdown("country", Number.parseInt(days, 10)),
    placeholderData: (previous) => previous,
  });

  // Merge the SSE ring buffer over the fetched trail so the newest entries
  // appear instantly while the polled slice provides the history.
  const merged = mergeFeed(recent.data ?? [], feed);

  useEffect(() => {
    // The warehouse sheet re-renders on every snapshot; keep the query cache
    // fresh so returning to this page is instant.
    return () => void queryClient.invalidateQueries({ queryKey: ["analytics"] });
  }, []);

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Analytics</h1>
        <p className="text-sm text-muted-foreground">
          The event warehouse: per-day aggregates and the raw trail.
        </p>
      </div>

      <ExplainerBanner
        title="How events reach ClickHouse"
        howTo={[
          "Every registration, login, OAuth link, and job transition emits a named event through an EventSink.",
          "With ClickHouse configured, events buffer in memory and flush in batches (500 rows or every 2 s) — never one insert per request.",
          "No warehouse? The dashboard degrades to operational counters; in dev mode the in-memory trail powers this page.",
        ]}
      >
        The operational counters (tiles below) come from PostgreSQL via filtered subqueries and stay
        correct even during a warehouse outage. Everything event-shaped — the bar chart, the trail,
        the live feed — comes from ClickHouse (or the in-memory trail when running without
        APP_CLICKHOUSE_ADDR). Live updates ride the same SSE stream as the dashboard.
      </ExplainerBanner>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard title="Events registered" value={summary.events_registered ?? 0} />
        <KpiCard title="Events logged in" value={summary.events_logged_in ?? 0} />
        <KpiCard title="Jobs queued (events)" value={summary.events_jobs_queued ?? 0} />
        <KpiCard title="Jobs failed (events)" value={summary.events_jobs_failed ?? 0} />
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-0">
            <CardTitle className="text-sm">Daily active users</CardTitle>
          </CardHeader>
          <CardContent>
            <ChartContainer config={chartConfig} className="h-48 w-full">
              <AreaChart data={dau.data ?? []} margin={{ left: -20, right: 12 }}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="day" tickLine={false} axisLine={false} minTickGap={40} />
                <YAxis tickLine={false} axisLine={false} width={40} />
                <Area
                  dataKey="count"
                  type="monotone"
                  fill="var(--color-registered)"
                  stroke="var(--color-registered)"
                  name="DAU"
                />
              </AreaChart>
            </ChartContainer>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-0">
            <CardTitle className="text-sm">Feature usage (events)</CardTitle>
          </CardHeader>
          <CardContent>
            <ChartContainer config={chartConfig} className="h-48 w-full">
              <BarChart
                data={featureBreakdown.data ?? []}
                layout="vertical"
                margin={{ left: 8, right: 12 }}
              >
                <CartesianGrid horizontal={false} />
                <XAxis type="number" tickLine={false} axisLine={false} />
                <YAxis
                  dataKey="name"
                  type="category"
                  tickLine={false}
                  axisLine={false}
                  width={80}
                />
                <Bar dataKey="count" fill="var(--color-user_logged_in)" radius={4} name="Events" />
              </BarChart>
            </ChartContainer>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-0">
          <CardTitle className="text-sm">Visitors by country (events)</CardTitle>
        </CardHeader>
        <CardContent>
          <ChartContainer config={chartConfig} className="h-40 w-full">
            <BarChart data={countryBreakdown.data ?? []} margin={{ left: -20, right: 12 }}>
              <CartesianGrid vertical={false} />
              <XAxis dataKey="name" tickLine={false} axisLine={false} />
              <YAxis tickLine={false} axisLine={false} width={40} />
              <Bar dataKey="count" fill="var(--color-oauth_linked)" radius={4} name="Events" />
            </BarChart>
          </ChartContainer>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <div>
            <CardTitle className="text-base">Daily events</CardTitle>
            <CardDescription>
              Count per event name, grouped by day, from the warehouse.
            </CardDescription>
          </div>
          <Select value={days} onValueChange={setDays}>
            <SelectTrigger className="w-28" aria-label="Chart window">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="7">7 days</SelectItem>
              <SelectItem value="14">14 days</SelectItem>
              <SelectItem value="30">30 days</SelectItem>
              <SelectItem value="90">90 days</SelectItem>
            </SelectContent>
          </Select>
        </CardHeader>
        <CardContent>
          <Tabs value={view} onValueChange={(value) => setView(value as EventView)}>
            <TabsList className="mb-3 flex-wrap">
              <TabsTrigger value="total">Total</TabsTrigger>
              {EVENT_NAMES.map((name) => (
                <TabsTrigger key={name} value={name}>
                  {chartConfig[name]?.label ?? name}
                </TabsTrigger>
              ))}
            </TabsList>
            <EventBars data={series.data ?? []} dataKey={view} />
          </Tabs>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Event trail</CardTitle>
            <CardDescription>Newest warehouse events with the acting user id.</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="max-h-72 space-y-1.5 overflow-y-auto text-sm">
              {merged.length === 0 ? (
                <li className="text-muted-foreground">
                  No events stored. Run the demo seed or configure ClickHouse, then interact with
                  the app.
                </li>
              ) : (
                merged.map((event, index) => (
                  <li
                    key={`${event.occurred_at}-${index}`}
                    className="flex items-center gap-2 border-b pb-1.5 last:border-0"
                  >
                    <span className="font-mono text-xs">{event.name}</span>
                    <span className="truncate font-mono text-xs text-muted-foreground">
                      {event.user_id || ""}
                    </span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {format(new Date(event.occurred_at), "MMM d, HH:mm:ss")}
                    </span>
                  </li>
                ))
              )}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Live feed (SSE)</CardTitle>
            <CardDescription>Streamed as events are emitted — no polling.</CardDescription>
          </CardHeader>
          <CardContent>
            {live.feed.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Idle. Register, log in, or run a job — even in another browser tab — and entries
                appear here instantly.
              </p>
            ) : (
              <ul className="max-h-72 space-y-1.5 overflow-y-auto text-sm">
                {live.feed.map((event, index) => (
                  <li key={`${event.occurred_at}-${index}`} className="flex items-center gap-2">
                    <span className="size-1.5 rounded-full bg-emerald-500" />
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
      </div>
    </div>
  );
}

// The chart renders one series at a time: "total" or a single event name.
type EventView = "total" | (typeof EVENT_NAMES)[number];

function EventBars({
  data,
  dataKey,
}: {
  data: { day: string; total: number; events: Record<string, number> }[];
  dataKey: string;
}) {
  return (
    <ChartContainer config={chartConfig} className="h-64 w-full">
      <BarChart data={data} margin={{ left: -20, right: 12 }}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="day" tickLine={false} axisLine={false} tickMargin={8} minTickGap={40} />
        <ChartTooltip content={<ChartTooltipContent indicator="line" />} />
        <Bar
          dataKey={dataKey}
          fill={dataKey === "total" ? "var(--color-total)" : `var(--color-${dataKey})`}
          radius={4}
        />
      </BarChart>
    </ChartContainer>
  );
}

function mergeFeed(
  trail: { name: string; occurred_at: string; user_id?: string }[],
  feed: typeof trail,
) {
  const merged = [...feed, ...trail];
  const seen = new Set<string>();
  return merged.filter((event) => {
    const key = `${event.name}:${event.occurred_at}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

const summaryQuery = {
  queryKey: ["analytics", "summary"],
  queryFn: api.analyticsSummary,
};
