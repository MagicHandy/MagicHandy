// The model check is backend-authoritative: the server decides each verdict
// and reports the numbers behind it; the browser only words them.
export type ModelCheckStatus = "pass" | "warn" | "fail" | "skip";

export type ModelCheckItemID =
  | "load"
  | "gpu"
  | "template"
  | "runtime"
  | "reasoning"
  | "contract"
  | "truncation"
  | "speed"
  | "verbosity"
  | "refusals";

export interface ModelCheckItem {
  id: ModelCheckItemID | string;
  status: ModelCheckStatus;
}

export interface ModelCheckTurn {
  message: string;
  reply?: string;
  motion?: string;
  millis: number;
  first_token_millis: number;
  valid_json: boolean;
  first_try: boolean;
  repaired: boolean;
  failed: boolean;
  truncated: boolean;
  refused: boolean;
  reasoning_chars: number;
  thinking_markup: boolean;
  words: number;
  decode_tokens: number;
  decode_millis: number;
  error?: string;
}

export interface ModelCheckSummary {
  turns: number;
  first_try: number;
  repaired: number;
  failed: number;
  truncated: number;
  refused: number;
  reasoning_chars: number;
  thinking_markup: number;
  average_millis: number;
  average_first_token_millis: number;
  average_words: number;
  tokens_per_second: number;
}

export interface ModelCheckReport {
  state: "idle" | "running" | "complete" | "failed" | "canceled";
  step?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
  provider?: string;
  model?: string;
  managed: boolean;
  reply_length?: string;
  load_millis: number;
  load_report: { offloaded_layers: number; total_layers: number };
  runtime?: { version?: string; expected: string; current: boolean };
  template?: {
    model_id: string;
    architecture?: string;
    fix?: string;
    fix_source?: "known" | "user" | string;
    fix_offer?: string;
  };
  items: ModelCheckItem[];
  turns: ModelCheckTurn[];
  summary: ModelCheckSummary;
}
