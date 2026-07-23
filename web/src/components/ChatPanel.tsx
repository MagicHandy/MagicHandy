// Streaming chat over the server-side shared message log (ADR 0003): history
// loads from the canonical log, other tabs' messages arrive via the state
// poll, and this client advances only its own cursor. Keeps near-bottom
// scroll stickiness with a jump-to-latest affordance and surfaces the
// malformed-response state. Chat can start, adjust, and stop motion through
// the backend contract; the frontend sends only text. When speak-replies is
// on, the controller tab (the audio-lease owner) plays completed TTS clips.
import { useCallback, useEffect, useRef, useState } from "react";
import { api, streamChat } from "../api/client";
import type { ChatMessageDiagnostics } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { useVoicePlayback } from "../state/voice-playback";
import { VoiceComposerControls } from "./VoiceComposerControls";

interface Msg {
  id: string;
  role: "user" | "assistant";
  text: string;
  streaming?: boolean;
  warning?: boolean;
  diagnostics?: ChatMessageDiagnostics;
}

interface Props {
  sessionId: string;
  onBusyChange?: (busy: boolean) => void;
  onSessionChanged?: () => void;
}

const uid = () => Math.random().toString(36).slice(2, 10);
const message = (error: unknown) => error instanceof Error ? error.message : "Conversation history request failed.";

export function ChatPanel({ sessionId, onBusyChange, onSessionChanged }: Props) {
  const { backendOnline, readOnly, state, refresh } = useAppState();
  const { show } = useToast();
  const { queueSpeech } = useVoicePlayback();
  const [messages, setMessages] = useState<Msg[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [showJump, setShowJump] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const lastSeq = useRef(0);
  const seeded = useRef(false);
  const mounted = useRef(false);
  const historyLoad = useRef<Promise<void> | null>(null);
  const tailLoad = useRef<Promise<void> | null>(null);
  const [historyLoading, setHistoryLoading] = useState(true);
  const [historyError, setHistoryError] = useState("");
  const [tailError, setTailError] = useState("");
  const [voiceActive, setVoiceActive] = useState(false);

  const loadHistory = useCallback(async () => {
    if (historyLoad.current) return historyLoad.current;
    setHistoryLoading(true);
    setHistoryError("");
    const request = (async () => {
      try {
        const res = await api.getChatMessages(sessionId);
        if (!mounted.current) return;
        seeded.current = true;
        setMessages(res.messages.map((m) => ({ id: `log-${m.seq}`, role: m.role, text: m.content, diagnostics: m.diagnostics })));
        lastSeq.current = res.latest_seq;
        setHistoryError("");
        if (res.latest_seq > res.cursor) void api.advanceChatCursor(sessionId, res.latest_seq).catch(() => undefined);
      } catch (error) {
        if (!mounted.current) return;
        seeded.current = false;
        setHistoryError(message(error));
      } finally {
        if (mounted.current) setHistoryLoading(false);
      }
    })();
    historyLoad.current = request;
    try {
      await request;
    } finally {
      if (historyLoad.current === request) historyLoad.current = null;
    }
  }, [sessionId]);

  const loadTail = useCallback(async () => {
    if (tailLoad.current || !seeded.current || busyRef.current) return tailLoad.current;
    const after = lastSeq.current;
    const request = (async () => {
      try {
        const res = await api.getChatMessages(sessionId, after);
        if (!mounted.current || busyRef.current) return;
        const fresh = res.messages.filter((m) => m.seq > lastSeq.current);
        if (!fresh.length) {
          setTailError(res.latest_seq > lastSeq.current ? "Conversation updates could not be synchronized; retrying." : "");
          return;
        }
        setMessages((m) => [...m, ...fresh.map((x) => ({ id: `log-${x.seq}`, role: x.role, text: x.content, diagnostics: x.diagnostics }))]);
        if (!readOnly) {
          for (const message of fresh) {
            if (message.speech_request_id) queueSpeech(message.speech_request_id);
          }
        }
        lastSeq.current = Math.max(lastSeq.current, res.latest_seq);
        setTailError("");
        void api.advanceChatCursor(sessionId, res.latest_seq).catch(() => undefined);
      } catch (error) {
        if (mounted.current) setTailError(`Conversation updates delayed: ${message(error)} Retrying.`);
      }
    })();
    tailLoad.current = request;
    try {
      await request;
    } finally {
      if (tailLoad.current === request) tailLoad.current = null;
    }
  }, [queueSpeech, readOnly, sessionId]);

  // Seed from the canonical log, then keep one tail request in flight. The
  // uptime dependency retries transient failures on the next backend poll even
  // when the latest sequence itself has not changed.
  useEffect(() => {
    mounted.current = true;
    void loadHistory();
    return () => {
      mounted.current = false;
      onBusyChange?.(false);
    };
  }, [loadHistory, onBusyChange]);

  const latestSeq = state?.chat?.active_session_id === sessionId ? state?.chat?.latest_seq ?? 0 : 0;
  const pollEpoch = state?.uptime_seconds ?? 0;
  useEffect(() => {
    if (busy || !seeded.current || latestSeq <= lastSeq.current) return;
    void loadTail();
  }, [busy, latestSeq, loadTail, pollEpoch]);

  useEffect(() => {
    if (stick.current) {
      const el = logRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }
  }, [messages]);

  function onScroll() {
    const el = logRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
    stick.current = nearBottom;
    setShowJump(!nearBottom);
  }
  function jump() {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
    stick.current = true;
    setShowJump(false);
  }

  const historyUnavailable = historyLoading || Boolean(historyError);
  const locked = !backendOnline || !state || readOnly || historyUnavailable;

  // Speech input shows only when it can work: voice on and an ASR provider
  // selected. A configured-but-stopped worker leaves the button disabled with
  // a pointer to Settings — workers are never started implicitly from here.
  const voiceSettings = state?.settings?.voice;
  const asrConfigured = Boolean(voiceSettings?.enabled && voiceSettings.asr_provider && voiceSettings.asr_provider !== "none");
  const asrWorker = state?.voice?.workers?.asr;
  const asrReady = asrWorker?.state === "running" && asrWorker.model_state === "ready";

  async function sendText(input: string, stopSequence?: number) {
    const text = input.trim();
    if (!text || busyRef.current || locked) return;
    const assistantId = uid();
    setMessages((m) => [
      ...m,
      { id: uid(), role: "user", text },
      { id: assistantId, role: "assistant", text: "", streaming: true },
    ]);
    busyRef.current = true;
    setBusy(true);
    onBusyChange?.(true);
    let raw = "";
    let repairRaw = "";
    let mustRefreshStopState = false;
    try {
      await streamChat(sessionId, text, (ev) => {
        if (ev.event === "status") {
          const status = ev.data as { state?: string; provider?: string; model?: string; prompt_set?: string; user_seq?: number; stop_sequence?: number };
          const userSeq = Number(status.user_seq ?? 0);
          if (userSeq > lastSeq.current) lastSeq.current = userSeq;
          if (status.state === "deterministic_stop") mustRefreshStopState = true;
          const statusDiagnostics: ChatMessageDiagnostics = {
            source: "interactive",
            provider: status.provider,
            model: status.model,
            prompt_set: status.prompt_set,
          };
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, diagnostics: statusDiagnostics } : x)));
        } else if (ev.event === "speech") {
          const requestId = String((ev.data as { request_id?: string }).request_id ?? "");
          if (requestId) queueSpeech(requestId);
        } else if (ev.event === "delta" || ev.event === "repair_delta") {
          const phase = (ev.data as { phase?: string }).phase;
          const chunk = (ev.data as { text?: string }).text ?? "";
          if (ev.event === "repair_delta" || phase === "repair") repairRaw += chunk;
          else raw += chunk;
          const draftReply = extractReplyDraft(ev.event === "repair_delta" ? repairRaw : raw);
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: draftReply || x.text || "..." } : x)));
        } else if (ev.event === "message") {
          const finalReply = String(ev.data.reply ?? "");
          const replySeq = Number((ev.data as { seq?: number }).seq ?? 0);
          if (replySeq > lastSeq.current) lastSeq.current = replySeq;
          setMessages((m) => m.map((x) => (x.id === assistantId ? {
            ...x,
            text: finalReply || "...",
            warning: Boolean(ev.data.initial_malformed),
            diagnostics: ev.data.diagnostics ?? x.diagnostics,
          } : x)));
        } else if (ev.event === "malformed") {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
        } else if (ev.event === "motion") {
          const motionError = String(ev.data.error ?? "").trim();
          if (motionError) {
            // A model-requested stop is not the global Emergency Stop, and a
            // rejected stop may still have reached the device, so the wording
            // stays neutral about both.
            const prefix = ev.data.action === "stop" ? "Device Stop could not be confirmed" : "Motion command failed";
            show(`${prefix}: ${motionError}`, "error");
            setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
          }
        } else if (ev.event === "error") {
          const message = String((ev.data as { message?: string }).message ?? "Chat error");
          // A reply the backend already committed stays visible: replacing it
          // with the error would contradict the history a reload shows.
          const replyRetained = String((ev.data as { reply_retained?: string }).reply_retained ?? "") === "true";
          show(message, "error");
          setMessages((m) => m.map((x) => (x.id === assistantId
            ? { ...x, text: replyRetained ? x.text : message, warning: true }
            : x)));
        } else if (ev.event === "done" && ev.data.ok === false) {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: x.text || "Malformed model response.", warning: true } : x)));
        }
      }, undefined, stopSequence);
      if (lastSeq.current > 0) void api.advanceChatCursor(sessionId, lastSeq.current).catch(() => undefined);
    } catch (e) {
      const message = e instanceof Error ? e.message : "Chat failed.";
      show(message, "error");
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true } : x)));
    } finally {
      // A deterministic Chat Stop advances the backend invalidation sequence.
      // Keep composition locked until the authoritative snapshot catches up.
      if (mustRefreshStopState) await refresh();
      else void refresh();
      busyRef.current = false;
      setBusy(false);
      onBusyChange?.(false);
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, streaming: false } : x)));
      onSessionChanged?.();
    }
  }

  async function send() {
    const text = draft.trim();
    if (!text) return;
    setDraft("");
    await sendText(text, state?.stop_sequence);
  }

  return (
    <div className="chat" id="active-chat-panel" role="tabpanel" aria-label="Active conversation">
      <div className="chat-log-shell">
        <div className="chat-log" ref={logRef} onScroll={onScroll} role="log" aria-live="polite" aria-relevant="additions" aria-busy={historyLoading || undefined}>
          {historyLoading && <div className="chat-history-state" role="status">Loading conversation…</div>}
          {historyError && (
            <div className="chat-history-state" role="alert">
              <strong>Conversation unavailable</strong>
              <span>{historyError}</span>
              <button type="button" className="btn btn-secondary" onClick={() => void loadHistory()}>Retry</button>
            </div>
          )}
          {!historyLoading && !historyError && messages.length === 0 && <div className="chat-history-state chat-history-empty">No messages yet</div>}
          {messages.map((m) => (
            <div key={m.id} className="chat-message" data-role={m.role} data-streaming={m.streaming || undefined} data-state={m.warning ? "warning" : undefined}>
              {m.role === "assistant" ? <AssistantAvatar message={m} /> : <span className="chat-avatar" aria-hidden="true">Y</span>}
              <div className="chat-body">
                <span className="chat-speaker">{m.role === "user" ? "You" : "MagicHandy"}</span>
                <div className="chat-bubble">{m.text || (m.warning ? "Malformed model JSON — the reply could not be parsed." : "")}</div>
              </div>
            </div>
          ))}
          {tailError && <p className="form-status chat-sync-status" role="status">{tailError}</p>}
        </div>
        {showJump && (
          <button type="button" className="btn btn-secondary chat-jump" onClick={jump}>Jump to latest</button>
        )}
      </div>
      <form
        className="chat-form"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        <div className="chat-compose-row" data-has-voice={asrConfigured || undefined}>
          {asrConfigured && (
            <VoiceComposerControls
              disabled={locked}
              ready={asrReady}
              unavailableTitle="Start and load the speech-input worker in Settings → Voice"
              preferences={{
                input_mode: voiceSettings?.input_mode ?? "hands_free",
                input_sensitivity: voiceSettings?.input_sensitivity ?? 55,
                input_silence_ms: voiceSettings?.input_silence_ms ?? 900,
                input_noise_suppression: voiceSettings?.input_noise_suppression ?? true,
              }}
              stopSequence={state?.stop_sequence}
              onActivityChange={setVoiceActive}
              onTranscript={sendText}
              showError={(message) => show(message, "error")}
            />
          )}
          <label className="visually-hidden" htmlFor="chat-input">Message</label>
          <textarea
            id="chat-input"
            rows={2}
            maxLength={1000}
            value={draft}
            disabled={locked || voiceActive}
            placeholder={historyError ? "Conversation history unavailable." : historyLoading ? "Loading conversation…" : readOnly ? "Read-only — this tab can't drive motion." : "Message MagicHandy…"}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                void send();
              }
            }}
          />
          <button type="submit" className="btn btn-primary chat-send" disabled={locked || busy || voiceActive || !draft.trim()}>Send</button>
        </div>
        <span className="visually-hidden" role="status">{busy ? "Streaming" : voiceActive ? "Voice input active" : historyError ? "Conversation history unavailable" : historyLoading ? "Loading conversation history" : locked ? (readOnly ? "Read-only" : "Core offline") : "Idle"}</span>
      </form>
    </div>
  );
}

function AssistantAvatar({ message }: { message: Msg }) {
  const diagnostics = message.diagnostics;
  if (!diagnostics || !Object.values(diagnostics).some((value) => value !== undefined && value !== "" && value !== false)) {
    return <span className="chat-avatar" aria-hidden="true">M</span>;
  }
  const tooltipID = `chat-diagnostics-${message.id.replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const rows = diagnosticRows(diagnostics);
  const title = rows.map(([label, value]) => `${label}: ${value}`).join("\n");
  return (
    <span className="chat-avatar-diagnostics">
      <button type="button" className="chat-avatar" aria-label="Show response diagnostics" aria-describedby={tooltipID} title={title}>M</button>
      <span id={tooltipID} className="chat-diagnostics-tooltip" role="tooltip">
        <strong>Response diagnostics</strong>
        <dl>
          {rows.map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </span>
    </span>
  );
}

function diagnosticRows(diagnostics: ChatMessageDiagnostics): Array<[string, string]> {
  const rows: Array<[string, string]> = [];
  if (diagnostics.source) rows.push(["Source", sourceLabel(diagnostics.source)]);
  if (diagnostics.provider) rows.push(["Provider", diagnostics.provider]);
  if (diagnostics.model) rows.push(["Model", diagnostics.model]);
  if (diagnostics.prompt_set) rows.push(["Prompt set", diagnostics.prompt_set]);
  if (Number.isFinite(diagnostics.request_ms)) rows.push(["Run time", `${Math.max(0, Math.round(diagnostics.request_ms ?? 0))} ms`]);
  if (diagnostics.motion_action) rows.push(["Motion", diagnostics.motion_action]);
  if (diagnostics.repaired) rows.push(["Parser", "Repaired response"]);
  if (diagnostics.semantic_fallback) rows.push(["Fallback", "Semantic fallback used"]);
  if (diagnostics.initial_malformed) rows.push(["Initial response", "Malformed JSON"]);
  return rows;
}

function sourceLabel(source: string): string {
  switch (source) {
    case "interactive": return "Interactive chat";
    case "autopilot": return "Autopilot";
    case "deterministic_stop": return "Deterministic Stop";
    default: return source;
  }
}

function extractReplyDraft(raw: string): string {
  const key = raw.indexOf('"reply"');
  if (key === -1) return "";
  const colon = raw.indexOf(":", key + 7);
  if (colon === -1) return "";
  const quote = raw.indexOf('"', colon + 1);
  if (quote === -1) return "";
  let value = "";
  let escaping = false;
  for (let index = quote + 1; index < raw.length; index += 1) {
    const character = raw[index];
    if (escaping) {
      value += decodeEscape(character);
      escaping = false;
      continue;
    }
    if (character === "\\") {
      escaping = true;
      continue;
    }
    if (character === '"') return value;
    value += character;
  }
  return value;
}

function decodeEscape(character: string): string {
  switch (character) {
    case "n":
      return "\n";
    case "r":
      return "\r";
    case "t":
      return "\t";
    case '"':
    case "\\":
    case "/":
      return character;
    default:
      return character;
  }
}
