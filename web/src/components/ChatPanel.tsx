// Streaming chat over the server-side shared message log (ADR 0003): history
// loads from the canonical log, other tabs' messages arrive via the state
// poll, and this client advances only its own cursor. Keeps near-bottom
// scroll stickiness with a jump-to-latest affordance and surfaces the
// malformed-response state. Chat can start, adjust, and stop motion through
// the backend contract; the frontend sends only text. When speak-replies is
// on, the controller tab (the audio-lease owner) plays completed TTS clips.
import { useEffect, useRef, useState } from "react";
import { api, streamChat } from "../api/client";
import type { ChatHistoryMessage, ChatLogMessage } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { audioPlaybackToken, playBlob } from "../util/audio";
import { VoiceComposerControls } from "./VoiceComposerControls";

interface Msg {
  id: string;
  role: "user" | "assistant";
  text: string;
  streaming?: boolean;
  warning?: boolean;
}

const uid = () => Math.random().toString(36).slice(2, 10);
const MAX_HISTORY = 12;

// The LLM context convention wraps assistant turns as the JSON contract body.
function toLlmHistory(message: ChatLogMessage): ChatHistoryMessage {
  if (message.role === "user") return { role: "user", content: message.content };
  return { role: "assistant", content: JSON.stringify({ reply: message.content, motion: { action: "none" } }) };
}

export function ChatPanel() {
  const { backendOnline, readOnly, state } = useAppState();
  const { show } = useToast();
  const [messages, setMessages] = useState<Msg[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [showJump, setShowJump] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const history = useRef<ChatHistoryMessage[]>([]);
  const lastSeq = useRef(0);
  const seeded = useRef(false);
  const speechQueue = useRef<string[]>([]);
  const speaking = useRef(false);
  const speechAbort = useRef<AbortController | null>(null);
  const speechGeneration = useRef(0);
  const [voiceActive, setVoiceActive] = useState(false);

  useEffect(() => {
    const cancelSpeech = () => {
      speechGeneration.current++;
      speechQueue.current = [];
      speechAbort.current?.abort();
      speechAbort.current = null;
    };
    window.addEventListener("magichandy:emergency-stop", cancelSpeech);
    return () => {
      window.removeEventListener("magichandy:emergency-stop", cancelSpeech);
      cancelSpeech();
    };
  }, []);

  // Seed the panel from the canonical server log once.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await api.getChatMessages();
        if (cancelled || seeded.current) return;
        seeded.current = true;
        if (res.messages.length > 0) {
          setMessages(res.messages.map((m) => ({ id: `log-${m.seq}`, role: m.role, text: m.content })));
        }
        history.current = res.messages.slice(-MAX_HISTORY).map(toLlmHistory);
        lastSeq.current = res.latest_seq;
        if (res.latest_seq > res.cursor) void api.advanceChatCursor(res.latest_seq).catch(() => undefined);
      } catch {
        // Core offline: the shell banner reports it; the panel stays empty.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Continuity with other tabs: when the shared log advances beyond what
  // this tab has displayed (and it is not mid-exchange), pull the tail.
  const latestSeq = state?.chat?.latest_seq ?? 0;
  useEffect(() => {
    if (busy || !seeded.current || latestSeq <= lastSeq.current) return;
    let cancelled = false;
    void (async () => {
      try {
        const res = await api.getChatMessages(lastSeq.current);
        if (cancelled || busy) return;
        const fresh = res.messages.filter((m) => m.seq > lastSeq.current);
        if (!fresh.length) return;
        setMessages((m) => [...m, ...fresh.map((x) => ({ id: `log-${x.seq}`, role: x.role, text: x.content }))]);
        history.current = [...history.current, ...fresh.map(toLlmHistory)].slice(-MAX_HISTORY);
        lastSeq.current = res.latest_seq;
        void api.advanceChatCursor(res.latest_seq).catch(() => undefined);
      } catch {
        // Retried on the next state poll.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [latestSeq, busy]);

  // Sequential playback of completed TTS clips. Only this (controller) tab
  // receives speech events, and the backend lease refuses everyone else, so
  // two tabs never speak the same clip.
  function queueSpeech(requestId: string) {
    speechQueue.current.push(requestId);
    void drainSpeech();
  }
  async function drainSpeech() {
    if (speaking.current) return;
    speaking.current = true;
    try {
      while (speechQueue.current.length) {
        const id = speechQueue.current.shift();
        if (!id) continue;
        const generation = speechGeneration.current;
        const done = await waitForSpeechDone(id);
        if (!done || generation !== speechGeneration.current) continue;
        try {
          const playbackToken = audioPlaybackToken();
          const abort = new AbortController();
          speechAbort.current = abort;
          const blob = await api.voiceRequestAudio(id, abort.signal);
          if (generation !== speechGeneration.current) continue;
          await playBlob(blob, playbackToken);
        } catch {
          // Missing/denied audio is not a chat failure.
        } finally {
          speechAbort.current = null;
        }
      }
    } finally {
      speaking.current = false;
    }
  }

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

  const locked = !backendOnline || !state || readOnly;

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
    let raw = "";
    let repairRaw = "";
    let finalReply = "";
    let finalMotion: Record<string, unknown> = { action: "none" };
    try {
      await streamChat(text, history.current, (ev) => {
        if (ev.event === "status") {
          const userSeq = Number((ev.data as { user_seq?: number }).user_seq ?? 0);
          if (userSeq > lastSeq.current) lastSeq.current = userSeq;
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
          finalReply = String(ev.data.reply ?? "");
          finalMotion = (ev.data.motion ?? { action: "none" }) as Record<string, unknown>;
          const replySeq = Number((ev.data as { seq?: number }).seq ?? 0);
          if (replySeq > lastSeq.current) lastSeq.current = replySeq;
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: finalReply || "...", warning: Boolean(ev.data.initial_malformed) } : x)));
        } else if (ev.event === "malformed") {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
        } else if (ev.event === "error") {
          const message = String((ev.data as { message?: string }).message ?? "Chat error");
          show(message, "error");
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true } : x)));
        } else if (ev.event === "done" && ev.data.ok === false && !finalReply) {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: x.text || "Malformed model response.", warning: true } : x)));
        }
      }, undefined, stopSequence);
      if (finalReply) {
        const nextHistory: ChatHistoryMessage[] = [
          ...history.current,
          { role: "user", content: text },
          { role: "assistant", content: JSON.stringify({ reply: finalReply, motion: finalMotion }) },
        ];
        history.current = nextHistory.slice(-MAX_HISTORY);
      }
      if (lastSeq.current > 0) void api.advanceChatCursor(lastSeq.current).catch(() => undefined);
    } catch (e) {
      const message = e instanceof Error ? e.message : "Chat failed.";
      show(message, "error");
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true } : x)));
    } finally {
      busyRef.current = false;
      setBusy(false);
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, streaming: false } : x)));
    }
  }

  async function send() {
    const text = draft.trim();
    if (!text) return;
    setDraft("");
    await sendText(text, state?.stop_sequence);
  }

  return (
    <div className="chat">
      <div className="chat-log-shell">
        <div className="chat-log" ref={logRef} onScroll={onScroll} role="log" aria-live="polite" aria-relevant="additions">
          {messages.map((m) => (
            <div key={m.id} className="chat-message" data-role={m.role} data-streaming={m.streaming || undefined} data-state={m.warning ? "warning" : undefined}>
              <span className="chat-avatar" aria-hidden="true">{m.role === "user" ? "Y" : "M"}</span>
              <div className="chat-body">
                <span className="chat-speaker">{m.role === "user" ? "You" : "MagicHandy"}</span>
                <div className="chat-bubble">{m.text || (m.warning ? "Malformed model JSON — the reply could not be parsed." : "")}</div>
              </div>
            </div>
          ))}
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
            placeholder={readOnly ? "Read-only — this tab can't drive motion." : "Message MagicHandy…"}
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
        <span className="visually-hidden" role="status">{busy ? "Streaming" : voiceActive ? "Voice input active" : locked ? (readOnly ? "Read-only" : "Core offline") : "Idle"}</span>
      </form>
    </div>
  );
}

// waitForSpeechDone polls a TTS request until it completes; false means it
// failed, was canceled, or timed out (nothing to play).
async function waitForSpeechDone(requestId: string): Promise<boolean> {
  const deadline = Date.now() + 30000;
  for (;;) {
    try {
      const res = await api.voiceRequest(requestId);
      const state = res.request?.state;
      if (state === "done") return (res.request?.audio_bytes ?? 0) > 0;
      if (state === "failed" || state === "canceled") return false;
    } catch {
      return false;
    }
    if (Date.now() > deadline) return false;
    await new Promise((resolve) => setTimeout(resolve, 400));
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
