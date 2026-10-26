// Collapsed technical facts for one voice worker: what a bug report needs,
// kept out of the way of the everyday status line. Raw backend text is shown
// verbatim here so nothing the backend reported is lost.
import { Children, type ReactNode } from "react";
import type { VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { formatNumber, t, translateKnown, type MessageKey } from "../i18n";

const REQUEST_TYPE: Partial<Record<string, MessageKey>> = {
  speak: "Synthesis",
  transcribe: "Transcription",
};

// Only finished requests are shown here; queued and active work is the
// voice queue's job.
const REQUEST_STATE: Partial<Record<string, MessageKey>> = {
  done: "Done",
  failed: "Failed",
  canceled: "Canceled",
};

const REJECTION: Partial<Record<string, MessageKey>> = {
  no_speech: "No speech detected",
  low_confidence: "Low confidence",
};

function localTime(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : value;
}

interface Props {
  worker?: VoiceWorkerStatus;
  module?: VoiceModuleStatus;
  lastRequest?: VoiceRequestSnapshot;
  error: string;
  output: string;
  children?: ReactNode;
}

export function VoiceWorkerDetails({ worker, module, lastRequest, error, output, children }: Props) {
  const service = [worker?.provider, worker?.provider_version].filter(Boolean).join(" ");
  const protocol = worker?.protocol_version ? formatNumber(worker.protocol_version) : "";
  const started = localTime(worker?.started_at);
  return (
    <details className="advanced-fields voice-details">
      <summary>{t("Technical details")}</summary>
      <dl className="voice-details-list">
        <div><dt>{t("Provider")}</dt><dd>{service || t("Unknown")}</dd></div>
        {protocol && <div><dt>{t("Protocol version")}</dt><dd>{protocol}</dd></div>}
        <div><dt>{t("Model")}</dt><dd>{worker?.model_state || t("Unknown")}</dd></div>
        <div><dt>{t("Requests waiting")}</dt><dd>{formatNumber(worker?.queue_depth ?? 0)}</dd></div>
        {started && <div><dt>{t("Running since")}</dt><dd>{started}</dd></div>}
        {module?.message && <div><dt>{t("Module")}</dt><dd>{translateKnown(module.message)}</dd></div>}
        {lastRequest && <div><dt>{t("Last request")}</dt><dd><LastRequest request={lastRequest} /></dd></div>}
        {error && <div><dt>{t("Last error")}</dt><dd className="voice-details-raw">{error}</dd></div>}
      </dl>
      {output && <>
        <p className="voice-details-caption">{t("Worker output")}</p>
        <pre className="voice-details-output">{output}</pre>
      </>}
      {Children.toArray(children).length > 0 && <div className="row-actions">{children}</div>}
    </details>
  );
}

function LastRequest({ request }: { request: VoiceRequestSnapshot }) {
  const kind = translateKnown(REQUEST_TYPE[request.type] ?? request.type);
  const state = translateKnown(REQUEST_STATE[request.state] ?? request.state);
  return (
    <span className="voice-details-request">
      <span>{kind} · {state} · {localTime(request.created_at)}</span>
      {request.first_audio_ms !== undefined && <span>{t("First audio after {ms} ms", { ms: formatNumber(request.first_audio_ms) })}</span>}
      {request.completion_ms !== undefined && <span>{t("Finished after {ms} ms", { ms: formatNumber(request.completion_ms) })}</span>}
      {request.audio_truncated && <span>{t("The audio was cut off at the size limit.")}</span>}
      {request.rejected && <span>{translateKnown(REJECTION[request.rejected] ?? request.rejected)}</span>}
      {request.error && <span className="voice-details-raw"><code>{request.error.code}</code> {request.error.message}</span>}
    </span>
  );
}
