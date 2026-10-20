export interface StartupReport {
  schema_version: number;
  app_version: string;
  commit: string;
  platform: string;
  started_at: string;
  finished_at: string;
  stage: string;
  outcome: "ready" | "recovered" | "failed";
  port_policy?: string;
  error_code?: string;
  native_code?: number;
  attempts: { listener: string; port: number; error_code?: string; native_code?: number }[];
}

export interface StartupReports {
  current: StartupReport | null;
  last_failure: StartupReport | null;
  last_recovery: StartupReport | null;
}
