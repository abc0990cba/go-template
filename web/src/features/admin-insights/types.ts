export interface AnalyticsSummary {
  total_users: number;
  password_identities: number;
  google_identities: number;
  github_identities: number;
  facebook_identities: number;
  twitter_identities: number;
  enrichment_succeeded: number;
  enrichment_failed: number;
  jobs_queued: number;
  jobs_done: number;
  jobs_failed: number;
  events_registered?: number;
  events_logged_in?: number;
  events_oauth_linked?: number;
  events_jobs_queued?: number;
  events_jobs_done?: number;
  events_jobs_failed?: number;
}

export interface EventRecord {
  name: string;
  occurred_at: string;
  user_id?: string;
}

export interface EventDayPoint {
  day: string;
  events: Record<string, number>;
  total: number;
}

export interface NameCount {
  name: string;
  count: number;
}

export interface DayCount {
  day: string;
  count: number;
}

export interface Report {
  id: string;
  status: "done" | "failed";
  result: string;
  error: string;
  created_at: string;
  updated_at: string;
}

export interface AuditEntry {
  id: string;
  actor_id: string;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  domain?: string;
  diff?: string;
  ip?: string;
  request_id?: string;
  created_at: string;
}
