import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { ChatMessage, OperationMode, StatusSnapshot, VoiceStatus } from "../api/types";
import { useToast } from "../contexts/ToastContext";
import { browserSttAvailable, listenPushToTalk } from "../lib/browserStt";

const PLAYBACK_FAILED = "PLAYBACK_FAILED";

function ttsOf(voice: VoiceStatus | null) {
  return voice?.tts ?? voice;
}

function sttOf(voice: VoiceStatus | null) {
  return voice?.stt;
}

async function playSpeechBlob(blob: Blob) {
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  await new Promise<void>((resolve, reject) => {
    audio.onended = () => {
      URL.revokeObjectURL(url);
      resolve();
    };
    audio.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(PLAYBACK_FAILED));
    };
    void audio.play().catch(reject);
  });
}

function roleLabel(role: string, t: (key: string) => string) {
  if (role === "assistant") return t("chat.roleAssistant");
  if (role === "user") return t("chat.roleUser");
  if (role === "system") return t("chat.roleSystem");
  return role;
}

export function Chat() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [lastPersona, setLastPersona] = useState("");
  const [text, setText] = useState("");
  const [mode, setMode] = useState<OperationMode>("hybrid");
  const [sending, setSending] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const [transcribing, setTranscribing] = useState(false);
  const [recording, setRecording] = useState(false);
  const [voice, setVoice] = useState<VoiceStatus | null>(null);
  const [status, setStatus] = useState<StatusSnapshot | null>(null);
  const [feedbackBusy, setFeedbackBusy] = useState(false);
  const lastSpokenRef = useRef("");
  const mediaRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);
  const browserSttRef = useRef<ReturnType<typeof listenPushToTalk> | null>(null);

  const tts = ttsOf(voice);
  const stt = sttOf(voice);

  const speakText = useCallback(
    async (content: string) => {
      if (!content.trim()) return;
      setSpeaking(true);
      try {
        const blob = await api.speakVoice(content);
        await playSpeechBlob(blob);
      } catch (e) {
        const msg =
          e instanceof Error && e.message === PLAYBACK_FAILED
            ? t("chat.playbackFailed")
            : e instanceof Error
              ? e.message
              : t("chat.ttsError");
        notify(msg, "error");
      } finally {
        setSpeaking(false);
      }
    },
    [notify, t],
  );

  const sendMessage = useCallback(
    async (msg: string) => {
      const clean = msg.trim();
      if (!clean) return;
      setSending(true);
      try {
        const res = await api.sendChat(clean);
        if (res.stopped) notify(t("chat.stopWordDetected"), "ok");
        else if (res.pending) notify(t("chat.sentBackground"), "ok");
        else if (res.reply) notify(t("chat.sentPlanner"), "ok");
        else notify(t("chat.sentPlanner"), "ok");
        return res;
      } catch (e) {
        notify(e instanceof Error ? e.message : t("chat.sendError"), "error");
        throw e;
      } finally {
        setSending(false);
      }
    },
    [notify, t],
  );

  const refresh = useCallback(async () => {
    try {
      const data = await api.getChatMessages();
      setMessages(data.messages.slice(-40));
      const persona = data.last_persona_message || "";
      setLastPersona(persona);
      if (
        tts?.enabled &&
        tts.auto_speak_after_chat &&
        tts.available &&
        persona &&
        persona !== lastSpokenRef.current
      ) {
        lastSpokenRef.current = persona;
        void speakText(persona);
      }
    } catch {
      /* ignore poll errors */
    }
  }, [speakText, tts]);

  useEffect(() => {
    refresh();
    api.getStatus().then((s) => {
      setMode(s.operation_mode);
      setStatus(s);
    }).catch(() => {});
    api.getVoiceStatus().then(setVoice).catch(() => {});
    const id = setInterval(() => {
      void refresh();
      api.getStatus().then(setStatus).catch(() => {});
    }, 3000);
    return () => clearInterval(id);
  }, [refresh]);

  const send = async () => {
    const msg = text.trim();
    if (!msg) {
      notify(t("chat.enterMessage"), "error");
      return;
    }
    setText("");
    await sendMessage(msg);
    await refresh();
  };

  const applyTranscript = useCallback(
    async (transcript: string) => {
      if (!transcript.trim()) {
        notify(t("chat.nothingRecognized"), "error");
        return;
      }
      if (stt?.auto_send) {
        await sendMessage(transcript);
        await refresh();
        notify(t("chat.voiceSent"), "ok");
      } else {
        setText((prev) => (prev ? `${prev} ${transcript}` : transcript));
        notify(t("chat.transcriptReady"), "ok");
      }
    },
    [notify, sendMessage, stt?.auto_send, refresh, t],
  );

  const stopBrowserListen = useCallback(async () => {
    const session = browserSttRef.current;
    if (!session) return;
    browserSttRef.current = null;
    setRecording(false);
    setTranscribing(true);
    session.stop();
    try {
      const transcript = await session.done;
      await applyTranscript(transcript);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("chat.sttBrowserError"), "error");
    } finally {
      setTranscribing(false);
    }
  }, [applyTranscript, notify, t]);

  const stopRecording = useCallback(async () => {
    const recorder = mediaRef.current;
    if (!recorder || recorder.state === "inactive") return;
    setRecording(false);
    await new Promise<void>((resolve) => {
      recorder.onstop = () => resolve();
      recorder.stop();
    });
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
    mediaRef.current = null;

    const blob = new Blob(chunksRef.current, { type: "audio/webm" });
    chunksRef.current = [];
    if (blob.size < 100) {
      notify(t("chat.recordingTooShort"), "error");
      return;
    }

    setTranscribing(true);
    try {
      const { text: transcript } = await api.transcribeVoice(blob);
      await applyTranscript(transcript);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("chat.sttError"), "error");
    } finally {
      setTranscribing(false);
    }
  }, [applyTranscript, notify, t]);

  const startRecording = async () => {
    if (!stt?.enabled) {
      notify(t("chat.enableStt"), "error");
      return;
    }
    if (stt.provider === "browser") {
      if (!browserSttAvailable()) {
        notify(t("chat.useChrome"), "error");
        return;
      }
      const session = listenPushToTalk(String(stt.language || "pt"));
      browserSttRef.current = session;
      setRecording(true);
      return;
    }
    if (!stt.available) {
      notify(t("chat.installVoice"), "error");
      return;
    }
    if (!navigator.mediaDevices?.getUserMedia) {
      notify(t("chat.micUnavailable"), "error");
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = (ev) => {
        if (ev.data.size > 0) chunksRef.current.push(ev.data);
      };
      mediaRef.current = recorder;
      recorder.start();
      setRecording(true);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("chat.micDenied"), "error");
    }
  };

  const toggleMic = () => {
    if (recording) {
      if (browserSttRef.current) void stopBrowserListen();
      else void stopRecording();
    } else {
      void startRecording();
    }
  };

  useEffect(() => {
    return () => {
      browserSttRef.current?.stop();
      streamRef.current?.getTracks().forEach((t) => t.stop());
    };
  }, []);

  const onMode = async (m: OperationMode) => {
    setMode(m);
    try {
      await api.setOperationMode(m);
      notify(t("chat.modeSet", { mode: m }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const sendPlaybackFeedback = async (feedback: "like" | "dislike") => {
    const blockId = status?.playback_current?.block_id;
    if (!blockId) return;
    setFeedbackBusy(true);
    try {
      const res = await api.patternFeedback(blockId, feedback);
      notify(
        feedback === "like"
          ? t("chat.feedbackLike", {
              name: status?.playback_current?.display_name ?? blockId,
            })
          : t("chat.feedbackDislike", {
              score: res.success_score?.toFixed(2) ?? "—",
            }),
        "ok",
      );
      const snap = await api.getStatus();
      setStatus(snap);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("chat.feedbackError"), "error");
    } finally {
      setFeedbackBusy(false);
    }
  };

  const ttsHint =
    tts && !tts.available && tts.enabled
      ? tts.install_hint || 'pip install -e ".[voice]"'
      : null;
  const sttHint =
    stt && !stt.available && stt.enabled
      ? stt.install_hint || 'pip install -e ".[voice]" + ffmpeg'
      : null;

  return (
    <div>
      <h2>{t("chat.title")}</h2>
      <div className="row" style={{ marginBottom: "1rem", flexWrap: "wrap" }}>
        <label className="muted">{t("chat.operationMode")}</label>
        <select
          value={mode}
          onChange={(e) => onMode(e.target.value as OperationMode)}
        >
          <option value="manual">{t("chat.modeManual")}</option>
          <option value="auto">{t("chat.modeAuto")}</option>
          <option value="hybrid">{t("chat.modeHybrid")}</option>
        </select>
        <button type="button" className="btn" onClick={send} disabled={sending}>
          {sending ? t("chat.sending") : t("chat.sendToPlanner")}
        </button>
        {stt?.enabled && (
          <button
            type="button"
            className={`btn ${recording ? "btn-danger" : "btn-ghost"}`}
            disabled={transcribing || !stt.available}
            onClick={toggleMic}
          >
            {transcribing
              ? t("chat.transcribing")
              : recording
                ? t("chat.stopRecording")
                : t("chat.speakMic")}
          </button>
        )}
      </div>

      {ttsHint && <p className="hint warn-text">{ttsHint}</p>}
      {sttHint && <p className="hint warn-text">{sttHint}</p>}

      {status?.playback_current?.block_id && (
        <div className="panel" style={{ marginBottom: "1rem" }}>
          <div className="label row" style={{ justifyContent: "space-between", flexWrap: "wrap" }}>
            <span>{t("chat.playingNow")}</span>
            <span className="row" style={{ gap: "0.35rem" }}>
              <button
                type="button"
                className="btn btn-sm"
                disabled={feedbackBusy}
                onClick={() => void sendPlaybackFeedback("like")}
              >
                👍
              </button>
              <button
                type="button"
                className="btn btn-sm btn-ghost"
                disabled={feedbackBusy}
                onClick={() => void sendPlaybackFeedback("dislike")}
              >
                👎
              </button>
            </span>
          </div>
          <p style={{ margin: "0.35rem 0 0" }}>
            <strong>{status.playback_current.display_name ?? status.playback_current.block_id}</strong>
          </p>
          {status.playback_current.semantic_summary && (
            <p className="hint" style={{ margin: "0.25rem 0 0" }}>
              {status.playback_current.semantic_summary}
            </p>
          )}
        </div>
      )}

      <div className="panel">
        <div className="label row" style={{ justifyContent: "space-between" }}>
          <span>{t("chat.personaResponse")}</span>
          {lastPersona && tts?.enabled && tts.available && (
            <button
              type="button"
              className="btn btn-sm btn-ghost"
              disabled={speaking}
              onClick={() => void speakText(lastPersona)}
            >
              {speaking ? t("chat.speaking") : t("chat.listen")}
            </button>
          )}
        </div>
        <p style={{ margin: "0.5rem 0 0" }}>
          {lastPersona || <span className="muted">{t("chat.noResponse")}</span>}
        </p>
      </div>

      <div className="messages">
        {messages.map((m) => (
          <div
            key={m.id}
            className={`message ${m.role === "assistant" ? "msg-assistant" : "msg-user"}`}
          >
            <div className="role row">
              <span>{roleLabel(m.role, t)}</span>
              {m.role === "assistant" &&
                tts?.enabled &&
                tts.available &&
                m.content && (
                  <button
                    type="button"
                    className="btn btn-sm btn-ghost"
                    disabled={speaking}
                    onClick={() => void speakText(m.content)}
                  >
                    {t("chat.listen")}
                  </button>
                )}
            </div>
            {m.content}
          </div>
        ))}
      </div>

      <div className="field">
        <label>{t("chat.messageLabel")}</label>
        <textarea
          rows={3}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              void send();
            }
          }}
          placeholder={t("chat.ctrlEnter")}
        />
      </div>
    </div>
  );
}
