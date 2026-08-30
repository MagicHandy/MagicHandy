import { t, translateKnown, type MessageKey } from "../i18n";
import { api } from "../api/client";
import type { VoiceRequestSnapshot } from "../api/types";
import { useToast } from "../state/app-state";

const ROLE_LABEL: Partial<Record<string, MessageKey>> = {
  asr: "Speech input (ASR)",
  tts: "Speech output (TTS)",
};

const TYPE_LABEL: Partial<Record<string, MessageKey>> = {
  speak: "Synthesis",
  transcribe: "Transcription",
};

export function VoiceRequestQueue({
  locked,
  requests,
  refresh,
}: {
  locked: boolean;
  requests: VoiceRequestSnapshot[];
  refresh: () => Promise<void>;
}) {
  const { show } = useToast();
  const active = requests
    .filter((request) => request.state === "queued" || request.state === "active")
    .sort((left, right) => {
      if (left.state !== right.state) return left.state === "active" ? -1 : 1;
      return Date.parse(left.created_at) - Date.parse(right.created_at);
    });

  if (active.length === 0) return null;

  async function cancel(requestId: string) {
    try {
      await api.voiceRequestCancel(requestId);
    } catch (error) {
      const reason = error instanceof Error ? error.message : "The voice request could not be canceled.";
      show(reason, "error");
    } finally {
      void refresh();
    }
  }

  return (
    <section className="voice-request-queue" aria-labelledby="voice-queue-title">
      <header className="voice-request-queue-header">
        <h3 id="voice-queue-title">{t("Voice queue")}</h3>
        <span className="voice-request-count" aria-label={active.length === 1 ? t("1 voice request waiting") : t("{count} voice requests waiting", { count: active.length })}>{active.length}</span>
      </header>
      <div className="voice-request-list" aria-live="polite">
        {active.map((request) => (
          <div key={request.id} className="voice-request-row">
            <span className="status-dot" data-state={request.state === "active" ? "active" : "pending"} />
            <span className="voice-request-description">
              <strong>{translateKnown(ROLE_LABEL[request.role] ?? request.role)}</strong>
              <span>{translateKnown(TYPE_LABEL[request.type] ?? request.type)} <code>#{request.id}</code></span>
            </span>
            <span className="voice-request-state">{translateKnown(request.state)}</span>
            <button type="button" className="btn btn-secondary voice-request-cancel" disabled={locked} onClick={() => void cancel(request.id)}>{t("Cancel")}</button>
          </div>
        ))}
      </div>
    </section>
  );
}
