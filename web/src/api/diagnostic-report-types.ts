export interface DiagnosticReportOptions {
  settings: boolean;
  traces: boolean;
  chat: "none" | "failures" | "full";
  model_output: boolean;
  personas: boolean;
  prompts: boolean;
  memories: boolean;
}

export const defaultDiagnosticReportOptions: DiagnosticReportOptions = {
  settings: true,
  traces: true,
  chat: "failures",
  model_output: false,
  personas: false,
  prompts: false,
  memories: false,
};
