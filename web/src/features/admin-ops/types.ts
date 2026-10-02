export type JobStatus = "queued" | "running" | "done" | "failed";
export type JobType = "user.enrich_batch" | "user.report_generate";

export interface Job {
  id: string;
  type: string;
  status: JobStatus;
  payload: string;
  error: string;
  created_at: string;
  updated_at: string;
}

export interface EnrichmentProfile {
  user_id: string;
  source: string;
  payload: string;
  created_at: string;
}

export interface OutboxEvent {
  id: string;
  event_type: string;
  aggregate_id: string;
  payload: string;
  created_at: string;
  processed_at?: string | null;
  attempts: number;
}

export interface DeadLetter {
  id: string;
  source: string;
  topic: string;
  payload: string;
  reason: string;
  attempts: number;
  created_at: string;
  replayed_at?: string | null;
}

export interface WebhookEndpoint {
  id: string;
  url: string;
  events: string[];
  description?: string;
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  id: string;
  endpoint_id: string;
  event: string;
  payload: string;
  status: "pending" | "success" | "failed";
  attempts: number;
  last_response_status: number;
  last_error: string;
  delivered_at: string | null;
  created_at: string;
}
