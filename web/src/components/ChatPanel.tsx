import { t, translateKnown } from "../i18n";
// Streaming chat over the server-side shared message log (ADR 0003): history
// loads from the canonical log, other tabs' messages arrive via the state
// poll, and this client advances only its own cursor. Keeps near-bottom
// scroll stickiness with a jump-to-latest affordance and surfaces the
// malformed-response state. Chat can start, adjust, and stop motion through
// the backend contract; the frontend sends only text. When speak-replies is
// on, the controller tab (the audio-lease owner) plays the ordered speech queue.
import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { streamChat, streamChatRepair } from "../api/client";
import type { ChatMessageDiagnostics, ChatResponseRepair, ChatStreamEvent } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { useRemoteChatSurface } from "../remote/RemoteExecutorProvider";
import { useVoicePlayback } from "../state/voice-playback";
import { VoiceComposerControls } from "./VoiceComposerControls";
import { useChatHistory, type ChatDisplayMessage } from "./useChatHistory";

type Msg = ChatDisplayMessage;

interface Props {
  sessionId: string;
  personaName?: string;
  onBusyChange?: (busy: boolean) => void;
  onSessionChanged?: () => void;
  /**
   * Beside an open video, the video's script or the viewer's Off choice can
   * own the device; replies are then words only (ADR 0032).
   */
  motionOwner?: "script" | "off";
  disabled?: boolean;
}

const uid = () => Math.random().toString(36).slice(2, 10);

export function ChatPanel({ sessionId, personaName, onBusyChange, onSessionChanged, motionOwner, disabled = false }: Props) {
  const { backendOnline, readOnly, remoteHeld, state, refresh } = useAppState();
  const { show } = useToast();
  const { queueSpeech } = useVoicePlayback();
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [showJump, setShowJump] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  const composerRef = useRef<HTMLTextAreaElement>(null);
  const repairFocusMessage = useRef<string>();
  const restoreRepairFocus = useRef(false);
  const stick = useRef(true);
  const mounted = useRef(false);
  const streamGeneration = useRef(0);
  const activeStream = useRef<AbortController | null>(null);
  const activeTurn = useRef({ started: false, stopRequested: false });
  const observedStopSequence = useRef(state?.stop_sequence);
  const observedEpoch = useRef(state?.observation?.epoch);
  const observedControllerGeneration = useRef(state?.controller?.generation);
  const observedMotionOwner = useRef(motionOwner);
  const [voiceActive, setVoiceActive] = useState(false);
  const activeChat = state?.chat?.active_session_id === sessionId ? state.chat : undefined;
  const {
    messages, setMessages, historyLoading, historyError, tailError, historyNotice,
    loadHistory, reconcileHistory, cancelReads, queueSpeechOnce,
  } = useChatHistory({
    sessionId, epoch: state?.observation?.epoch, backendOnline, readOnly, remoteHeld, busy, busyRef,
    latestSeq: activeChat?.latest_seq ?? 0, revision: activeChat?.revision,
    pollEpoch: state?.uptime_seconds ?? 0, queueSpeech,
  });

  function clearRepairOffers() {
    setMessages(current => current.some(message => message.repair?.available) ? current.map(message => message.repair?.available
      ? { ...message, repair: undefined, repairState: "unavailable" } : message) : current);
  }

  useEffect(() => {
    mounted.current = true;
    return () => {
      streamGeneration.current += 1;
      mounted.current = false;
      activeStream.current?.abort();
      activeStream.current = null;
      busyRef.current = false;
      onBusyChange?.(false);
    };
  }, [onBusyChange]);

  // Stop cancels the active reply once its stream has begun. The first event
  // names the request it answers, so a Stop pressed earlier waits for it and
  // the notice stays under that request instead of beside a duplicate of it.
  // The backend ends a canceled turn within moments; never wait long.
  function cancelForStop() {
    const controller = activeStream.current;
    const turn = activeTurn.current;
    if (controller && !turn.started) {
      if (!turn.stopRequested) window.setTimeout(() => controller.abort("Chat canceled by Emergency Stop."), 2000);
      turn.stopRequested = true;
    } else controller?.abort("Chat canceled by Emergency Stop.");
    clearRepairOffers();
  }

  useEffect(() => {
    const previous = observedStopSequence.current;
    const current = state?.stop_sequence;
    observedStopSequence.current = current;
    if (previous !== undefined && current !== undefined && previous !== current) cancelForStop();
  }, [state?.stop_sequence]);

  // This tab's own Stop acts at once. Waiting for the next state snapshot let
  // the backend end the stream first, which the browser reported only as its
  // own network error.
  useEffect(() => {
    window.addEventListener("magichandy:emergency-stop", cancelForStop);
    return () => window.removeEventListener("magichandy:emergency-stop", cancelForStop);
  }, []);

  useEffect(() => {
    const previous = observedEpoch.current;
    observedEpoch.current = state?.observation?.epoch;
    const authority = state?.controller;
    const previousGeneration = observedControllerGeneration.current;
    observedControllerGeneration.current = authority?.generation;
    const protectedTurn = authority?.heartbeat_required && authority.active && !authority.read_only;
    const ownershipChanged = previousGeneration !== undefined && previousGeneration !== authority?.generation;
    // A delayed status/heartbeat response locks new input, but is not proof that
    // an already-admitted streamed turn lost its server-side lease. Keep that
    // request alive until its own connection fails or authority is revoked.
    // The backend still fences inference/dispatch at Stop, expiry and takeover.
    if (previous !== state?.observation?.epoch || ownershipChanged || authority?.read_only ||
        (authority?.heartbeat_required && !authority.active) || ((!backendOnline || readOnly) && !protectedTurn)) {
      const lostControl = ownershipChanged || authority?.read_only || (authority?.heartbeat_required && !authority.active);
      activeStream.current?.abort(previous === state?.observation?.epoch && lostControl
        ? "The reply was interrupted because this browser lost control of the session. Take control, then try again."
        : "The reply was interrupted before it finished. Please try again.");
      clearRepairOffers();
    }
  }, [backendOnline, readOnly, state?.observation?.epoch, state?.controller]);

  useEffect(() => {
    if (observedMotionOwner.current !== motionOwner) { activeStream.current?.abort("The model or motion settings changed while the reply was being prepared. Please send your request again."); clearRepairOffers(); }
    observedMotionOwner.current = motionOwner;
  }, [motionOwner]);

  // A tab its phone remote holds stays awake while hidden, so its reply keeps
  // streaming (ADR 0056).
  const held = useRef(remoteHeld);
  useLayoutEffect(() => { held.current = remoteHeld; });
  useEffect(() => {
    const hidden = () => { if (document.visibilityState === "hidden" && !held.current) activeStream.current?.abort(); };
    document.addEventListener("visibilitychange", hidden);
    return () => document.removeEventListener("visibilitychange", hidden);
  }, []);

  useEffect(() => {
    if (stick.current) {
      const el = logRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }
  }, [messages]);

  useEffect(() => {
    if (!busy && restoreRepairFocus.current) {
      restoreRepairFocus.current = false;
      if (document.activeElement === document.body) composerRef.current?.focus({ preventScroll: true });
    }
  }, [busy]);

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
  const locked = disabled || !backendOnline || !state || readOnly || historyUnavailable;
  const assistantName = personaName?.trim() || "MagicHandy";

  // A new canonical turn invalidates the live offer. The backend owns all
  // context validation; this view never reconstructs eligibility from settings.
  const hasRepair = messages.some(message => message.repair?.available);
  useEffect(() => {
    if (!hasRepair) return;
    setMessages(current => current.map(message => message.repair?.available && message.sourceUserSequence && (activeChat?.latest_seq ?? 0) > message.sourceUserSequence
      ? { ...message, repair: undefined, repairState: "unavailable" }
      : message));
  }, [hasRepair, activeChat?.latest_seq, setMessages]);

  // Speech input shows only when it can work: voice on and an ASR provider
  // selected. A configured-but-stopped worker leaves the button disabled with
  // a pointer to Settings — workers are never started implicitly from here.
  const voiceSettings = state?.settings?.voice;
  const asrConfigured = Boolean(voiceSettings?.enabled && voiceSettings.asr_provider && voiceSettings.asr_provider !== "none");
  const asrWorker = state?.voice?.workers?.asr;
  const asrReady = asrWorker?.state === "running" && asrWorker.model_state === "ready";

  // A phone remote sends through this composer's path, so its message is
  // fenced by the same Stop sequence and spoken by this tab (ADR 0032).
  const remoteSend = useRef<(text: string, stopSequence: number) => boolean>(() => false);
  useLayoutEffect(() => {
    remoteSend.current = (text, stopSequence) => {
      if (locked || busyRef.current || voiceActive || !text.trim() || stopSequence !== state?.stop_sequence) return false;
      void sendText(text, stopSequence);
      return true;
    };
  });
  const remoteReady = !locked && !voiceActive;
  const remoteChat = useMemo(() => ({
    sessionId, personaName: assistantName, busy, ready: remoteReady, send: (text: string, stopSequence: number) => remoteSend.current(text, stopSequence),
    latestSeq: activeChat?.latest_seq, revision: activeChat?.revision,
  }), [sessionId, assistantName, busy, remoteReady, activeChat?.latest_seq, activeChat?.revision]);
  useRemoteChatSurface(remoteChat);

  async function sendText(input: string, stopSequence?: number, repairMessage?: Msg) {
    const text = input.trim();
    if ((!text && !repairMessage) || busyRef.current || locked) return;
    if (repairMessage && (!repairMessage.repair?.available ||
      Boolean(repairMessage.sourceUserSequence && (activeChat?.latest_seq ?? 0) > repairMessage.sourceUserSequence))) return;
    cancelReads();
    const userId = uid();
    const assistantId = repairMessage?.id ?? uid();
    setMessages((m) => {
      const previous = m.map(item => ({ ...item, repair: undefined }));
      return repairMessage ? previous.map(item => item.id === assistantId ? { ...item, streaming: true, repairState: "repairing" } : item)
        : [...previous, { id: userId, role: "user" as const, text }, { id: assistantId, role: "assistant" as const, text: "", streaming: true }].slice(-202);
    });
    busyRef.current = true;
    setBusy(true);
    onBusyChange?.(true);
    const controller = new AbortController();
    const requestGeneration = ++streamGeneration.current;
    activeStream.current = controller;
    // A repair continues a reply whose request is already known.
    const turn = { started: Boolean(repairMessage), stopRequested: false };
    activeTurn.current = turn;
    let raw = "";
    let repairRaw = "";
    let initialReply = "";
    let retainReplyOnAbort = false;
    let stagedReplySeen = false;
    let mustRefreshStopState = false;
    let rejectionSeen = false;
    let repairAttempted = Boolean(repairMessage);
    let userSequence = 0;
    try {
      const receive = (ev: ChatStreamEvent) => {
        if (!mounted.current || streamGeneration.current !== requestGeneration || controller.signal.aborted) return;
        turn.started = true;
        handleEvent(ev);
        if (turn.stopRequested) controller.abort("Chat canceled by Emergency Stop.");
      };
      const handleEvent = (ev: ChatStreamEvent) => {
        if (ev.event === "status") {
          const status = ev.data as {
            state?: string;
            provider?: string;
            model?: string;
            prompt_set?: string;
            persona_id?: string;
            persona_name?: string;
            user_seq?: number;
            stop_sequence?: number;
            refusal_fallback_from?: string;
            refusal_fallback_model?: string;
          };
          const userSeq = Number(status.user_seq ?? 0);
          if (Number.isSafeInteger(userSeq) && userSeq > 0) userSequence = userSeq;
          if (Number.isSafeInteger(userSeq) && userSeq > 0) setMessages((m) => m.map((x) => x.id === userId ? { ...x, seq: userSeq } : x));
          if (status.state === "deterministic_stop") mustRefreshStopState = true;
          if (status.state === "retrying_local") { raw = ""; repairRaw = ""; initialReply = ""; }
          if (status.state === "repairing_response") {
            repairAttempted = true;
            setMessages((m) => m.map((x) => x.id === assistantId ? { ...x, text: repairMessage ? x.text : "", warning: Boolean(repairMessage), repair: undefined, repairState: "repairing" } : x));
          }
          const statusDiagnostics: ChatMessageDiagnostics = {
            source: "interactive",
            provider: status.provider,
            model: status.model,
            prompt_set: status.prompt_set,
            persona_id: status.persona_id,
            persona_name: status.persona_name,
            refusal_fallback_from: status.refusal_fallback_from || (status.state === "retrying_local" ? "provider" : undefined),
            refusal_fallback_model: status.refusal_fallback_model,
          };
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, diagnostics: statusDiagnostics } : x)));
        } else if (ev.event === "speech") {
          const requestId = String((ev.data as { request_id?: string }).request_id ?? "");
          if (requestId) queueSpeechOnce(requestId);
        } else if (ev.event === "delta" || ev.event === "repair_delta") {
          const phase = (ev.data as { phase?: string }).phase;
          const chunk = (ev.data as { text?: string }).text ?? "";
          const repairing = ev.event === "repair_delta" || phase === "repair";
          // A manual repair keeps the original rejection visible until the
          // backend accepts a replacement, instead of exposing another draft.
          if (repairAttempted && repairing) return;
          if (repairing) repairRaw += chunk;
          else raw += chunk;
          const draftReply = extractReplyDraft(repairing ? repairRaw : raw);
          if (!repairing && draftReply) initialReply = draftReply;
          const visibleDraft = repairing && initialReply ? initialReply : draftReply;
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: visibleDraft || x.text || "..." } : x)));
        } else if (ev.event === "message") {
          const finalReply = String(ev.data.reply ?? "");
          const replySeq = Number((ev.data as { seq?: number }).seq ?? 0);
          stagedReplySeen = true;
          setMessages((m) => m.map((x) => (x.id === assistantId ? {
            ...x,
            seq: Number.isSafeInteger(replySeq) && replySeq > 0 ? replySeq : x.seq,
            text: finalReply || "...",
            warning: Boolean(ev.data.initial_malformed) && !repairAttempted,
            repair: undefined,
            repairState: undefined,
            responseRejected: false,
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
            const prefix = ev.data.action === "stop" ? t("Device Stop could not be confirmed") : t("Motion command failed");
            show(t("{state}: {message}", { state: prefix, message: motionError }), "error");
            setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
          }
        } else if (ev.event === "error") {
          rejectionSeen = true;
          const message = translateKnown(String((ev.data as { message?: string }).message ?? "Chat error"));
          const offer = (ev.data as { repair?: ChatResponseRepair }).repair;
          const repair = offer?.available && Number.isSafeInteger(offer.failure_id) && offer.failure_id > 0 ? offer : undefined;
          // A reply the backend already committed stays visible: replacing it
          // with the error would contradict the history a reload shows.
          const replyRetained = String((ev.data as { reply_retained?: string }).reply_retained ?? "") === "true";
          if (replyRetained) retainReplyOnAbort = true;
          if (!repair && !repairAttempted) show(message, "error");
          setMessages((m) => m.map((x) => (x.id === assistantId
            ? { ...x, text: replyRetained ? x.text : message, warning: true, repair,
                sourceUserSequence: userSequence || x.sourceUserSequence, responseRejected: !replyRetained && Boolean(repair),
                repairState: repairAttempted ? "failed" : undefined }
            : x)));
        } else if (ev.event === "done" && ev.data.ok === false) {
          const malformed = Boolean(ev.data.malformed);
          setMessages((m) => m.map((x) => (x.id === assistantId ? {
            ...x,
            text: malformed && !rejectionSeen ? t("Malformed model response.") : x.text || t("Malformed model response."),
            warning: true,
            repairState: repairAttempted ? "failed" : x.repairState,
          } : x)));
        }
      };
      if (repairMessage?.repair) await streamChatRepair(sessionId, repairMessage.repair.failure_id, receive, controller.signal, stopSequence, motionOwner);
      else await streamChat(sessionId, text, receive, controller.signal, stopSequence, motionOwner);
    } catch (e) {
      if (controller.signal.aborted || !mounted.current || streamGeneration.current !== requestGeneration) return;
      // A dropped connection rejects with the browser's own TypeError text,
      // such as "network error"; explain it in the app's words instead.
      const message = turn.stopRequested ? t("Chat canceled by Emergency Stop.")
        : e instanceof TypeError ? t("The reply was interrupted before it finished. Please try again.")
        : e instanceof Error ? translateKnown(e.message) : t("Chat failed.");
      if (!repairAttempted) show(message, "error");
      const staleRepair = typeof e === "object" && e !== null && "status" in e && e.status === 409;
      // Anchored to its request, so later replies cannot move this failure.
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true, repair: undefined, responseRejected: false,
        sourceUserSequence: userSequence || x.sourceUserSequence,
        repairState: repairAttempted ? staleRepair ? "unavailable" : "failed" : undefined } : x)));
    } finally {
      if (activeStream.current === controller) activeStream.current = null;
      if (!mounted.current || streamGeneration.current !== requestGeneration) return;
      // A deterministic Chat Stop advances the backend invalidation sequence.
      // Keep composition locked until the authoritative snapshot catches up.
      if (!controller.signal.aborted) {
        if (mustRefreshStopState) await refresh();
        else void refresh();
      }
      if (!mounted.current || streamGeneration.current !== requestGeneration) return;
      busyRef.current = false;
      setBusy(false);
      onBusyChange?.(false);
      setMessages((m) => m.flatMap((x) => {
        if (x.id !== assistantId) return [x];
        if (controller.signal.aborted && !retainReplyOnAbort) return repairMessage
          ? [{ ...repairMessage, streaming: false, repair: undefined, repairState: "unavailable" }]
          : [{ id: x.id, role: "assistant", streaming: false, warning: true, sourceUserSequence: userSequence || x.sourceUserSequence,
            text: typeof controller.signal.reason === "string" ? translateKnown(controller.signal.reason)
              : t("The reply was interrupted before it finished. Please try again.") }];
        return [{ ...x, streaming: false }];
      }));
      if (controller.signal.aborted && stagedReplySeen && !retainReplyOnAbort) {
        await loadHistory();
      } else if (!controller.signal.aborted) {
        // Reconcile every committed message since the last delivered page,
        // including replies that committed below the SSE display sequence.
        await reconcileHistory();
      }
      if (!mounted.current || streamGeneration.current !== requestGeneration) return;
      if (!controller.signal.aborted) onSessionChanged?.();
    }
  }

  async function send() {
    const text = draft.trim();
    if (!text || busyRef.current || locked || voiceActive) return;
    setDraft("");
    await sendText(text, state?.stop_sequence);
  }

  return (
    <div className="chat" id="active-chat-panel" role="tabpanel" aria-label={t("Active conversation")}>
      <div className="chat-log-shell">
        <div className="chat-log" ref={logRef} onScroll={onScroll} role="log" aria-live="polite" aria-relevant="additions" aria-busy={historyLoading || undefined}>
          {historyLoading && <div className="chat-history-state" role="status">{t("Loading conversation…")}</div>}
          {historyError && (
            <div className="chat-history-state" role="alert">
              <strong>{t("Conversation unavailable")}</strong>
              <span>{translateKnown(historyError)}</span>
              <button type="button" className="btn btn-secondary" onClick={() => void loadHistory()}>{t("Retry")}</button>
            </div>
          )}
          {!historyLoading && !historyError && messages.length === 0 && <div className="chat-history-state chat-history-empty">{t("No messages yet")}</div>}
          {messages.map((m, index) => {
            const divider = personaDivider(messages, index);
            return (
              <Fragment key={m.id}>
                {divider && <div className="persona-divider" role="separator">{divider}</div>}
                <div className="chat-message" data-role={m.role} data-streaming={m.streaming || undefined} data-state={m.warning ? "warning" : undefined}>
                  {m.role === "assistant" ? <AssistantAvatar message={m} /> : <span className="chat-avatar" aria-hidden="true">Y</span>}
                  <div className="chat-body">
                    <span className="chat-speaker">
                      {m.role === "user" ? t("You") : m.responseRejected || m.repairState ? "MagicHandy" : m.diagnostics?.persona_name || "MagicHandy"}
                    </span>
                    {(m.text || m.repairState !== "repairing") && <div className="chat-bubble">{m.responseRejected ? t("The model returned an invalid response. That response was not applied.") : m.text || (m.warning ? t("Malformed model JSON — the reply could not be parsed.") : "")}</div>}
                    {(m.repair?.available || m.repairState || m.responseRejected) && <div className="chat-repair-row">
                      {m.repair?.available && <button type="button" className="btn btn-secondary btn-sm" disabled={locked || busy || voiceActive} onClick={() => { repairFocusMessage.current = m.id; restoreRepairFocus.current = true; void sendText("", state?.stop_sequence, m); }}>{t("Repair response")}</button>}
                      {m.repairState && <span className="form-status" tabIndex={m.repairState === "repairing" ? -1 : undefined} ref={element => { if (element && m.repairState === "repairing" && repairFocusMessage.current === m.id) { repairFocusMessage.current = undefined; element.focus({ preventScroll: true }); } }}>{m.repairState === "repairing" ? t("Repairing response…") : m.repairState === "failed" ? t("Repair did not succeed. You can send a new request.") : t("Repair is no longer available. Send a new request.")}</span>}
                      {m.responseRejected && m.repairState !== "repairing" && <a href="#/settings/diagnostics" className="btn btn-quiet btn-sm">{t("View details in Diagnostics")}</a>}
                    </div>}
                    {m.diagnostics?.refusal_fallback_from && <p className="form-status chat-sync-status" role={m.streaming ? "status" : undefined}>{m.streaming ? t("Provider declined; retrying with the local model…") : t("Local retry after a provider refusal.")}</p>}
                    {m.contentDownload && <p className="form-status chat-sync-status">{t("Long message preview.")} {" "}
                      <a href={m.contentDownload} download>{t("Download full message")}</a>
                    </p>}
                  </div>
                </div>
              </Fragment>
            );
          })}
          {tailError && <p className="form-status chat-sync-status" role="status">{tailError}</p>}
          {historyNotice && <p className="form-status chat-sync-status" role="status">{historyNotice}</p>}
        </div>
        {showJump && (
          <button type="button" className="btn btn-secondary chat-jump" onClick={jump}>{t("Jump to latest")}</button>
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
              unavailableTitle={asrWorker?.state === "starting" || asrWorker?.model_state === "loading" ? t("Loading...") : t("Start and load the speech-input worker in Settings → Voice")}
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
          <label className="visually-hidden" htmlFor="chat-input">{t("Message")}</label>
          <textarea
            ref={composerRef}
            id="chat-input"
            rows={2}
            maxLength={1000}
            value={draft}
            disabled={locked || voiceActive}
            placeholder={historyError ? t("Conversation history unavailable.") : historyLoading ? t("Loading conversation…") : readOnly ? t("Read-only — this tab can't drive motion.") : t("Message {name}…", { name: assistantName })}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                void send();
              }
            }}
          />
          <button type="submit" className="btn btn-primary chat-send" disabled={locked || busy || voiceActive || !draft.trim()}>{t("Send")}</button>
        </div>
        <span className="visually-hidden" role="status">{busy ? messages.some(message => message.repairState === "repairing") ? t("Repairing response…") : t("Streaming") : voiceActive ? t("Voice input active") : historyError ? t("Conversation history unavailable") : historyLoading ? t("Loading conversation history") : locked ? (readOnly ? t("Read-only") : t("Core offline")) : t("Idle")}</span>
      </form>
    </div>
  );
}

function AssistantAvatar({ message }: { message: Msg }) {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const tooltipRef = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const diagnostics = message.diagnostics;
  useLayoutEffect(() => {
    if (!open) return;
    const position = () => {
      const button = buttonRef.current;
      const tooltip = tooltipRef.current;
      if (!button || !tooltip) return;
      const anchor = button.getBoundingClientRect();
      tooltip.style.left = `${Math.max(8, Math.min(anchor.right + 8, window.innerWidth - tooltip.offsetWidth - 8))}px`;
      // Leave the persistent Stop/navigation region reachable on small screens.
      tooltip.style.top = `${Math.max(8, Math.min(anchor.bottom - tooltip.offsetHeight, window.innerHeight - tooltip.offsetHeight - 96))}px`;
    };
    position();
    window.addEventListener("resize", position);
    document.addEventListener("scroll", position, true);
    return () => {
      window.removeEventListener("resize", position);
      document.removeEventListener("scroll", position, true);
    };
  }, [open, diagnostics]);
  const initial = message.responseRejected || message.repairState ? "M" : Array.from(diagnostics?.persona_name?.trim() || "MagicHandy")[0]?.toUpperCase() || "M";
  if (!diagnostics || !Object.values(diagnostics).some((value) => value !== undefined && value !== "" && value !== false)) {
    return <span className="chat-avatar" aria-hidden="true">{initial}</span>;
  }
  const tooltipID = `chat-diagnostics-${message.id.replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const rows = diagnosticRows(diagnostics);
  const title = rows.map(([label, value]) => `${label}: ${value}`).join("\n");
  return (
    <span className="chat-avatar-diagnostics" onMouseEnter={() => setOpen(true)} onMouseLeave={() => { if (document.activeElement !== buttonRef.current) setOpen(false); }}>
      <button ref={buttonRef} type="button" className="chat-avatar" aria-label={t("Show response diagnostics")} aria-describedby={open ? tooltipID : undefined} title={title} onFocus={() => setOpen(true)} onBlur={() => setOpen(false)}>{initial}</button>
      {open && createPortal(<span ref={tooltipRef} id={tooltipID} className="chat-diagnostics-tooltip" role="tooltip">
        <strong>{t("Response diagnostics")}</strong>
        <dl>
          {rows.map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </span>, document.body)}
    </span>
  );
}

function diagnosticRows(diagnostics: ChatMessageDiagnostics): Array<[string, string]> {
  const rows: Array<[string, string]> = [];
  if (diagnostics.source) rows.push([t("Source"), sourceLabel(diagnostics.source)]);
  if (diagnostics.provider) rows.push([t("Provider"), diagnostics.provider]);
  if (diagnostics.model) rows.push([t("Model"), diagnostics.model]);
  if (diagnostics.prompt_set) rows.push([t("Prompt set"), diagnostics.prompt_set]);
  if (diagnostics.persona_name) rows.push([t("Persona"), diagnostics.persona_name]);
  if (Number.isFinite(diagnostics.request_ms)) rows.push([t("Run time"), `${Math.max(0, Math.round(diagnostics.request_ms ?? 0))} ms`]);
  if (Number.isFinite(diagnostics.first_token_ms)) rows.push([t("First token"), `${Math.max(0, Math.round(diagnostics.first_token_ms ?? 0))} ms`]);
  if ((diagnostics.first_activity_ms ?? 0) > 0) rows.push([t("First provider activity"), `${diagnostics.first_activity_ms} ms`]);
  if ((diagnostics.model_load_ms ?? 0) > 0) rows.push([t("Model load"), `${diagnostics.model_load_ms} ms`]);
  if ((diagnostics.prompt_eval_ms ?? 0) > 0) rows.push([t("Prompt processing"), `${diagnostics.prompt_eval_ms} ms`]);
  if ((diagnostics.prompt_bytes ?? 0) > 0) rows.push([t("Input bytes / budget"), `${diagnostics.prompt_bytes} / ${diagnostics.prompt_limit_bytes}`]);
  if ((diagnostics.history_messages_dropped ?? 0) > 0) rows.push([t("History messages omitted"), String(diagnostics.history_messages_dropped)]);
  if ((diagnostics.prompt_tokens ?? 0) > 0) rows.push([t("Tokens (input / output)"), `${diagnostics.prompt_tokens} / ${diagnostics.generated_tokens ?? 0}`]);
  if ((diagnostics.cached_prompt_tokens ?? 0) > 0) rows.push([t("Cached input tokens"), String(diagnostics.cached_prompt_tokens)]);
  if (Number.isFinite(diagnostics.preparation_ms)) rows.push([t("Preparation"), `${Math.max(0, Math.round(diagnostics.preparation_ms ?? 0))} ms`]);
  if (Number.isFinite(diagnostics.scheduler_wait_ms)) rows.push([t("Model queue"), `${Math.max(0, Math.round(diagnostics.scheduler_wait_ms ?? 0))} ms`]);
  if (Number.isFinite(diagnostics.generation_ms)) rows.push([t("Generation"), `${Math.max(0, Math.round(diagnostics.generation_ms ?? 0))} ms`]);
  if (Number.isFinite(diagnostics.repair_ms) && (diagnostics.repair_ms ?? 0) > 0) rows.push([t("Repair"), `${Math.max(0, Math.round(diagnostics.repair_ms ?? 0))} ms`]);
  if ((diagnostics.provider_calls ?? 0) > 1) rows.push([t("Provider calls"), String(diagnostics.provider_calls)]);
  if (diagnostics.refusal_fallback_from) rows.push([t("Declined provider"), [diagnostics.refusal_fallback_from, diagnostics.refusal_fallback_model].filter(Boolean).join(" · ")]);
  if ((diagnostics.declined_request_ms ?? 0) > 0) rows.push([t("Before local retry"), `${diagnostics.declined_request_ms} ms`]);
  if (diagnostics.motion_action) rows.push([t("Motion"), translateKnown(diagnostics.motion_action)]);
  if (diagnostics.repaired) rows.push([t("Parser"), t("Repaired response")]);
  if (diagnostics.semantic_fallback) rows.push([t("Fallback"), t("Semantic fallback used")]);
  if (diagnostics.initial_malformed) rows.push([t("Initial response"), t("Malformed JSON")]);
  return rows;
}

function personaDivider(messages: Msg[], index: number): string {
  const current = messages[index];
  if (current.role !== "assistant" || current.streaming) return "";
  let previous: Msg | undefined;
  for (let cursor = index - 1; cursor >= 0; cursor -= 1) {
    if (messages[cursor].role === "assistant" && !messages[cursor].streaming) {
      previous = messages[cursor];
      break;
    }
  }
  if (!previous) return "";
  const previousID = previous.diagnostics?.persona_id ?? "";
  const currentID = current.diagnostics?.persona_id ?? "";
  if (previousID === currentID) return "";
  return currentID
    ? t("Persona changed to {name}", { name: current.diagnostics?.persona_name || t("Unknown persona") })
    : t("Persona cleared; using Settings");
}

function sourceLabel(source: string): string {
  switch (source) {
    case "interactive": return t("Interactive chat");
    case "autopilot": return t("Autopilot");
    case "deterministic_stop": return t("Deterministic Stop");
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
