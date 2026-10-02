import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";

/** Endpoints of the admin-insights slice: analytics, reports, audit. */
export function analyticsSummary() {
  return http.get<AnalyticsSummary>("/api/analytics/summary");
}

export function analyticsTimeseries(days: number) {
  return http
    .get<{ days: number; data: EventDayPoint[] }>(`/api/analytics/timeseries?days=${days}`)
    .then((r) => r.data);
}

export function recentEvents(limit: number) {
  return http.get<Paged<EventRecord>>(`/api/analytics/events?limit=${limit}`).then((p) => p.data);
}

export function analyticsBreakdown(by: string, days: number) {
  return http
    .get<{ days: number; data: NameCount[] }>(`/api/analytics/breakdown?by=${by}&days=${days}`)
    .then((r) => r.data);
}

export function analyticsDAU(days: number) {
  return http
    .get<{ days: number; data: DayCount[] }>(`/api/analytics/dau?days=${days}`)
    .then((r) => r.data);
}

export function listReports(params: Partial<ListParams>) {
  return http.get<Paged<Report>>(`/api/reports?${buildListQuery(params)}`);
}

export function getReport(id: string) {
  return http.get<Report>(`/api/reports/${encodeURIComponent(id)}`);
}

export function generateReport() {
  return http.post<Job>("/api/reports");
}

export function listAudit(
  params: Partial<ListParams> & {
    actor?: string;
    action?: string;
    resource?: string;
    resource_id?: string;
  },
) {
  const base = buildListQuery(params);
  const extra = new URLSearchParams();
  if (params.actor) extra.set("actor", params.actor);
  if (params.action) extra.set("action", params.action);
  if (params.resource) extra.set("resource", params.resource);
  if (params.resource_id) extra.set("resource_id", params.resource_id);
  const suffix = extra.toString();
  return http.get<Paged<AuditEntry>>(`/api/audit?${suffix ? base + "&" + suffix : base}`);
}

import type { Job } from "@/features/admin-ops";
import type {
  AnalyticsSummary,
  AuditEntry,
  DayCount,
  EventDayPoint,
  EventRecord,
  NameCount,
  Report,
} from "./types";
