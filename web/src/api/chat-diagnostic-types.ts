import type { ChatMessageDiagnostics } from "./types";

export interface ChatRequestFailure {
  id: number;
  created_at: string;
  error: string;
  kind?: string;
  motion_mode: string;
  reasoning_mode: string;
  max_tokens: number;
  app_version: string;
  app_commit: string;
  diagnostics: ChatMessageDiagnostics;
  content?: {
    raw_response: string;
    repair_response?: string;
    system_prompt: string;
    response_schema: string;
    truncated: boolean;
  };
}
