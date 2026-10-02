import { api, unwrap } from "@/shared/api/core";
import type { Paged, ListParams } from "@/shared/api/pagination";
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

/** Endpoints of the admin-insights slice: analytics, reports, audit. Every
 * call goes through the typed client, so paths, bodies, and responses are
 * checked against the OpenAPI contract at compile time. */

export function analyticsSummary(): Promise<AnalyticsSummary> {
  return unwrap(api.GET("/api/analytics/summary"));
}

export function analyticsTimeseries(days: number): Promise<EventDayPoint[]> {
  return unwrap(api.GET("/api/analytics/timeseries", { params: { query: { days } } })).then(
    (p) => p.data,
  );
}

export function recentEvents(limit: number): Promise<EventRecord[]> {
  return unwrap(api.GET("/api/analytics/events", { params: { query: { limit } } })).then(
    (p) => p.data,
  );
}

export function analyticsBreakdown(
  by: "feature" | "source" | "country",
  days: number,
): Promise<NameCount[]> {
  return unwrap(api.GET("/api/analytics/breakdown", { params: { query: { by, days } } })).then(
    (p) => p.data ?? [],
  );
}

export function analyticsDAU(days: number): Promise<DayCount[]> {
  return unwrap(api.GET("/api/analytics/dau", { params: { query: { days } } })).then(
    (p) => p.data ?? [],
  );
}

export function listReports(params: Partial<ListParams>): Promise<Paged<Report>> {
  /** The spec vocabulary for reports: the shared list params minus the
   * status/type/user_id filters reports do not take. */
  return unwrap(
    api.GET("/api/reports", {
      params: {
        query: {
          mode: params.mode,
          limit: params.limit,
          offset: params.offset,
          after: params.after,
          q: params.q,
        },
      },
    }),
  );
}

export function getReport(id: string): Promise<Report> {
  return unwrap(api.GET("/api/reports/{id}", { params: { path: { id } } }));
}

export function generateReport(): Promise<Job> {
  return unwrap(api.POST("/api/reports"));
}

export function listAudit(
  params: Partial<ListParams> & {
    actor?: string;
    action?: string;
    resource?: string;
    resource_id?: string;
  },
): Promise<Paged<AuditEntry>> {
  /** The spec vocabulary for audit: paging plus the four trail filters. */
  return unwrap(
    api.GET("/api/audit", {
      params: {
        query: {
          limit: params.limit,
          offset: params.offset,
          actor: params.actor,
          action: params.action,
          resource: params.resource,
          resource_id: params.resource_id,
        },
      },
    }),
  );
}
