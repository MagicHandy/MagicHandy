export interface AccessAuditEvent {
  id: string;
  sequence: number;
  occurred_at_ms: number;
  kind: string;
  outcome: string;
  actor: { type: "account" | "system" | "local" | "public"; account_id?: string; session_id?: string };
  target_account_id?: string;
  target_session_id?: string;
  grant_id?: string;
  operation?: string;
  epoch?: string;
  generation?: number;
  stop_sequence?: number;
  trace_sequence?: number;
  correlation?: string;
  http_status?: number;
  count?: number;
  expires_at_ms?: number;
}

export interface AccessAuditPage {
  events: AccessAuditEvent[];
  newest_sequence: number;
  oldest_sequence: number;
  next_before: number;
  has_more: boolean;
  retention_days: number;
  limit: number;
  row_limit: number;
  writer: {
    queue_depth: number;
    queue_limit: number;
    dropped_since_startup: number;
    write_failures_since_startup: number;
    storage_available: boolean;
    last_write_ms: number;
  };
}
