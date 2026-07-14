import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { MicrophoneIcon } from "../shell/icons";
import { recordingToWAV } from "../util/recording";

const RECORDING_LIMIT_SECONDS = 30;
const WARM_MICROPHONE_MS = 60_000;
const UPLOAD_TIMEOUT_MS = 30_000;
const VOICE_STOP_EVENT = "magichandy:emergency-stop";

type CaptureMode = "hands-free" | "hold";

interface VoiceComposerControlsProps {
  disabled: boolean;
  ready: boolean;
  unavailableTitle: string;
  stopSequence?: number;
  onActivityChange: (active: boolean) => void;
  onTranscript: (transcript: string, stopSequence?: number) => Promise<void>;
  showError: (message: string) => void;
}

interface AudioInput {
  deviceId: string;
  label: string;
}

export function VoiceComposerControls({
  disabled,
  ready,
  unavailableTitle,
  stopSequence,
  onActivityChange,
  onTranscript,
  showError,
}: VoiceComposerControlsProps) {
  const [mode, setMode] = useState<CaptureMode>("hands-free");
  const [menuOpen, setMenuOpen] = useState(false);
  const [selectedDevice, setSelectedDevice] = useState("default");
  const [audioInputs, setAudioInputs] = useState<AudioInput[]>([]);
  const [arming, setArming] = useState(false);
  const [recording, setRecording] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [microphoneReady, setMicrophoneReady] = useState(false);
  const [recordingSecondsLeft, setRecordingSecondsLeft] = useState(RECORDING_LIMIT_SECONDS);
  const mounted = useRef(true);
  const wantsRecording = useRef(false);
  const recorder = useRef<MediaRecorder | null>(null);
  const discardedRecorders = useRef(new WeakSet<MediaRecorder>());
  const microphoneStream = useRef<MediaStream | null>(null);
  const decoderContext = useRef<AudioContext | null>(null);
  const activeRequestID = useRef("");
  const activeUpload = useRef<AbortController | null>(null);
  const captureSession = useRef(0);
  const recordingTimer = useRef<number | null>(null);
  const countdownTimer = useRef<number | null>(null);
  const warmTimer = useRef<number | null>(null);
  const lastStopSequence = useRef<number | undefined>(undefined);
  const stopSequenceRef = useRef(stopSequence);
  const selectedDeviceRef = useRef(selectedDevice);
  const onTranscriptRef = useRef(onTranscript);
  const menuRef = useRef<HTMLDivElement>(null);

  onTranscriptRef.current = onTranscript;
  stopSequenceRef.current = stopSequence;
  selectedDeviceRef.current = selectedDevice;
  const active = arming || recording || processing;

  useEffect(() => onActivityChange(active), [active, onActivityChange]);

  useEffect(() => {
    mounted.current = true;
    const emergencyStop = () => abortCapture();
    window.addEventListener(VOICE_STOP_EVENT, emergencyStop);
    return () => {
      mounted.current = false;
      window.removeEventListener(VOICE_STOP_EVENT, emergencyStop);
      abortCapture();
      onActivityChange(false);
    };
  }, []);

  useEffect(() => {
    if (stopSequence === undefined) return;
    if (lastStopSequence.current !== undefined && stopSequence !== lastStopSequence.current) abortCapture();
    lastStopSequence.current = stopSequence;
  }, [stopSequence]);

  useEffect(() => {
    if (!ready) abortCapture();
  }, [ready]);

  useEffect(() => {
    if (disabled && active) abortCapture();
  }, [disabled, active]);

  useEffect(() => {
    if (!menuOpen) return;
    void refreshAudioInputs();
    const onPointerDown = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [menuOpen]);

  useEffect(() => {
    const mediaDevices = navigator.mediaDevices;
    if (!mediaDevices?.addEventListener) return;
    const onDeviceChange = () => void refreshAudioInputs();
    mediaDevices.addEventListener("devicechange", onDeviceChange);
    return () => mediaDevices.removeEventListener("devicechange", onDeviceChange);
  }, []);

  async function refreshAudioInputs() {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const devices = (await navigator.mediaDevices.enumerateDevices()).filter((device) => device.kind === "audioinput");
      setAudioInputs(devices.map((device, index) => ({
        deviceId: device.deviceId,
        label: device.label || `Microphone ${index + 1}`,
      })));
      const selected = selectedDeviceRef.current;
      if (selected !== "default" && !devices.some((device) => device.deviceId === selected)) {
        abortCapture();
        setSelectedDevice("default");
      }
    } catch {
      // Device enumeration can be denied before microphone permission.
    }
  }

  async function acquireMicrophone(session: number): Promise<MediaStream | null> {
    const current = microphoneStream.current;
    if (current?.getAudioTracks().some((track) => track.readyState === "live")) return current;
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
      showError("Microphone capture requires localhost or HTTPS in a supported browser.");
      return null;
    }

    setArming(true);
    try {
      const audio: MediaTrackConstraints | boolean = selectedDevice === "default"
        ? true
        : { deviceId: { exact: selectedDevice } };
      const stream = await navigator.mediaDevices.getUserMedia({ audio });
      if (!mounted.current || !wantsRecording.current || session !== captureSession.current) {
        stream.getTracks().forEach((track) => track.stop());
        return null;
      }
      microphoneStream.current = stream;
      setMicrophoneReady(true);
      for (const track of stream.getAudioTracks()) {
        track.addEventListener("ended", () => {
          if (microphoneStream.current !== stream) return;
          abortCapture();
          setSelectedDevice("default");
          if (mounted.current) showError("The selected microphone became unavailable.");
        }, { once: true });
      }
      decoderContext.current ??= new AudioContext();
      void refreshAudioInputs();
      return stream;
    } catch (error) {
      releaseMicrophone();
      if (mounted.current) showError(error instanceof Error ? error.message : "Microphone permission was denied.");
      return null;
    } finally {
      if (mounted.current) setArming(false);
    }
  }

  async function startRecording() {
    if (disabled || !ready || active || recorder.current) return;
    wantsRecording.current = true;
    clearWarmTimer();
    const session = ++captureSession.current;
    const captureStopSequence = stopSequenceRef.current;
    const stream = await acquireMicrophone(session);
    if (!stream || !mounted.current) {
      wantsRecording.current = false;
      return;
    }
    if (!wantsRecording.current || session !== captureSession.current) {
      releaseMicrophone();
      return;
    }

    try {
      const preferred = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus"]
        .find((mime) => MediaRecorder.isTypeSupported(mime));
      const nextRecorder = preferred ? new MediaRecorder(stream, { mimeType: preferred }) : new MediaRecorder(stream);
      const chunks: Blob[] = [];
      recorder.current = nextRecorder;
      nextRecorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunks.push(event.data);
      };
      nextRecorder.onerror = () => {
        discardedRecorders.current.add(nextRecorder);
        showError("The browser could not record from the selected microphone.");
        stopRecording(true);
      };
      nextRecorder.onstart = () => {
        if (!mounted.current || nextRecorder !== recorder.current) return;
        setArming(false);
        setRecording(true);
        startRecordingTimers();
      };
      nextRecorder.onstop = () => {
        if (recorder.current === nextRecorder) recorder.current = null;
        clearRecordingTimers();
        scheduleWarmRelease();
        if (mounted.current) setRecording(false);
        const discarded = discardedRecorders.current.has(nextRecorder);
        if (discarded) {
          if (mounted.current) setProcessing(false);
          return;
        }
        void finishRecording(nextRecorder.mimeType || preferred || "audio/webm", chunks, session, captureStopSequence);
      };
      nextRecorder.start(100);
    } catch (error) {
      recorder.current = null;
      wantsRecording.current = false;
      releaseMicrophone();
      if (mounted.current) showError(error instanceof Error ? error.message : "The browser could not start microphone recording.");
    }
  }

  function stopRecording(discard = false) {
    wantsRecording.current = false;
    clearRecordingTimers();
    const current = recorder.current;
    if (current && current.state !== "inactive") {
      if (discard) discardedRecorders.current.add(current);
      else if (mounted.current) setProcessing(true);
      current.stop();
    }
    if (mounted.current) {
      setArming(false);
      setRecording(false);
    }
  }

  async function finishRecording(mimeType: string, chunks: Blob[], session: number, captureStopSequence?: number) {
    let handedOffToChat = false;
    const blob = new Blob(chunks, { type: mimeType });
    if (blob.size === 0 || !mounted.current || session !== captureSession.current) {
      if (mounted.current) setProcessing(false);
      scheduleWarmRelease();
      return;
    }
    setProcessing(true);
    try {
      const context = decoderContext.current ?? new AudioContext();
      decoderContext.current = context;
      const wav = await recordingToWAV(blob, context);
      if (!mounted.current || session !== captureSession.current) return;
      const upload = new AbortController();
      activeUpload.current = upload;
      const uploadTimer = window.setTimeout(() => upload.abort(), UPLOAD_TIMEOUT_MS);
      let submitted;
      try {
        submitted = await api.voiceTranscribe(wav, "wav", captureStopSequence, upload.signal);
      } catch (error) {
        if (upload.signal.aborted && session === captureSession.current) throw new Error("Transcription upload timed out.");
        throw error;
      } finally {
        window.clearTimeout(uploadTimer);
        if (activeUpload.current === upload) activeUpload.current = null;
      }
      activeRequestID.current = submitted.request.id;
      const transcript = await waitForTranscript(submitted.request.id, () => session !== captureSession.current);
      activeRequestID.current = "";
      if (!mounted.current || session !== captureSession.current) return;
      if (!transcript) {
        showError("No speech was recognized.");
        return;
      }
      if (mounted.current && session === captureSession.current) {
        setProcessing(false);
        scheduleWarmRelease();
      }
      handedOffToChat = true;
      await onTranscriptRef.current(transcript, captureStopSequence);
    } catch (error) {
      if (mounted.current && session === captureSession.current) {
        showError(error instanceof Error ? error.message : "Transcription failed.");
      }
    } finally {
      activeRequestID.current = "";
      if (mounted.current && session === captureSession.current) setProcessing(false);
      if (!handedOffToChat) scheduleWarmRelease();
    }
  }

  function abortCapture() {
    captureSession.current++;
    wantsRecording.current = false;
    const requestID = activeRequestID.current;
    activeRequestID.current = "";
    activeUpload.current?.abort();
    activeUpload.current = null;
    if (requestID) void api.voiceRequestCancel(requestID).catch(() => undefined);
    stopRecording(true);
    releaseMicrophone();
    if (mounted.current) {
      setArming(false);
      setRecording(false);
      setProcessing(false);
    }
  }

  function releaseMicrophone() {
    clearWarmTimer();
    microphoneStream.current?.getTracks().forEach((track) => track.stop());
    microphoneStream.current = null;
    if (mounted.current) setMicrophoneReady(false);
    const context = decoderContext.current;
    decoderContext.current = null;
    if (context) void context.close().catch(() => undefined);
  }

  function startRecordingTimers() {
    clearRecordingTimers();
    setRecordingSecondsLeft(RECORDING_LIMIT_SECONDS);
    const startedAt = Date.now();
    countdownTimer.current = window.setInterval(() => {
      const left = RECORDING_LIMIT_SECONDS - Math.floor((Date.now() - startedAt) / 1000);
      setRecordingSecondsLeft(Math.max(0, left));
    }, 1000);
    recordingTimer.current = window.setTimeout(() => stopRecording(), RECORDING_LIMIT_SECONDS * 1000);
  }

  function clearRecordingTimers() {
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    if (countdownTimer.current !== null) window.clearInterval(countdownTimer.current);
    recordingTimer.current = null;
    countdownTimer.current = null;
  }

  function scheduleWarmRelease() {
    if (warmTimer.current !== null) return;
    if (!microphoneStream.current || !mounted.current) return;
    warmTimer.current = window.setTimeout(() => {
      warmTimer.current = null;
      if (!recorder.current) releaseMicrophone();
    }, WARM_MICROPHONE_MS);
  }

  function clearWarmTimer() {
    if (warmTimer.current !== null) window.clearTimeout(warmTimer.current);
    warmTimer.current = null;
  }

  function selectMode(nextMode: CaptureMode) {
    if (active) return;
    setMode(nextMode);
  }

  function selectInput(deviceId: string) {
    if (active || deviceId === selectedDevice) return;
    releaseMicrophone();
    setSelectedDevice(deviceId);
  }

  const label = arming
    ? "Cancel microphone startup"
    : recording
      ? `Stop and transcribe, ${recordingSecondsLeft} seconds remaining`
      : processing
        ? "Transcribing"
        : mode === "hands-free"
          ? "Start hands-free voice"
          : "Hold to talk";
  const title = ready
    ? microphoneReady
      ? `${label}. Microphone is warmed for faster starts.`
      : label
    : unavailableTitle;

  return (
    <div className="voice-input" ref={menuRef} data-open={menuOpen || undefined}>
      <div className="voice-input-control">
        <button
          type="button"
          className="voice-mic-button"
          data-active={recording || undefined}
          data-state={arming ? "arming" : processing ? "processing" : microphoneReady ? "ready" : undefined}
          disabled={disabled || processing || !ready}
          aria-label={label}
          aria-pressed={recording}
          title={title}
          onClick={mode === "hands-free" ? () => arming ? abortCapture() : recording ? stopRecording() : void startRecording() : undefined}
          onPointerDown={mode === "hold" ? (event) => {
            event.preventDefault();
            event.currentTarget.setPointerCapture(event.pointerId);
            void startRecording();
          } : undefined}
          onPointerUp={mode === "hold" ? (event) => {
            event.preventDefault();
            stopRecording();
          } : undefined}
          onPointerCancel={mode === "hold" ? () => stopRecording() : undefined}
          onKeyDown={mode === "hold" ? (event) => {
            if ((event.key === " " || event.key === "Enter") && !event.repeat) {
              event.preventDefault();
              void startRecording();
            }
          } : undefined}
          onKeyUp={mode === "hold" ? (event) => {
            if (event.key === " " || event.key === "Enter") {
              event.preventDefault();
              stopRecording();
            }
          } : undefined}
        >
          <MicrophoneIcon size={24} />
          {recording && <span className="voice-mic-countdown" aria-hidden="true">{recordingSecondsLeft}</span>}
          {active && (
            <span className="voice-capture-label" aria-hidden="true">
              {arming ? "Starting microphone" : processing ? "Transcribing" : "Listening"}
            </span>
          )}
        </button>
        <button
          type="button"
          className="voice-menu-trigger"
          aria-label={menuOpen ? "Close voice input menu" : "Open voice input menu"}
          aria-controls="voice-input-menu"
          aria-expanded={menuOpen}
          disabled={active}
          onClick={() => setMenuOpen((open) => !open)}
        >
          <span className="voice-menu-triangle" aria-hidden="true" />
        </button>
      </div>

      <div id="voice-input-menu" className="voice-input-menu" hidden={!menuOpen} aria-label="Voice input options">
        <span className="field-label">Voice mode</span>
        <div className="voice-mode-toggle" aria-label="Voice mode">
          <button type="button" aria-pressed={mode === "hold"} onClick={() => selectMode("hold")}>Hold to talk</button>
          <button type="button" aria-pressed={mode === "hands-free"} onClick={() => selectMode("hands-free")}>Hands-free</button>
        </div>
        <label className="field-label" htmlFor="voice-input-device">Voice input</label>
        <select
          id="voice-input-device"
          value={selectedDevice}
          disabled={active}
          onChange={(event) => selectInput(event.target.value)}
        >
          <option value="default">Default microphone</option>
          {audioInputs.filter((input) => input.deviceId && input.deviceId !== "default").map((input) => (
            <option key={input.deviceId} value={input.deviceId}>{input.label}</option>
          ))}
        </select>
        {microphoneReady && <button type="button" className="voice-release-button" disabled={active} onClick={abortCapture}>Release microphone</button>}
      </div>
    </div>
  );
}

async function waitForTranscript(requestID: string, canceled: () => boolean): Promise<string> {
  const deadline = Date.now() + 90_000;
  for (;;) {
    if (canceled()) {
      await api.voiceRequestCancel(requestID).catch(() => undefined);
      return "";
    }
    const res = await api.voiceRequest(requestID);
    const request = res.request;
    if (request.role !== "asr" || request.type !== "transcribe") throw new Error("The voice worker returned the wrong request.");
    if (request.state === "done") return request.transcript?.[0]?.text?.trim() ?? "";
    if (request.state === "failed") throw new Error(request.error?.message || "Transcription failed.");
    if (request.state === "canceled") return "";
    if (Date.now() > deadline) {
      await api.voiceRequestCancel(requestID).catch(() => undefined);
      throw new Error("Transcription timed out.");
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
}
