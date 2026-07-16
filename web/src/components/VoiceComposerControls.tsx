import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { MicrophoneIcon } from "../shell/icons";
import { encodePCM16WAV, recordingToWAV } from "../util/recording";
import { VoiceActivitySegmenter } from "../util/voice-activity";
import { openPCMStream, type PCMStream } from "../util/voice-capture";

const HOLD_RECORDING_LIMIT_SECONDS = 30;
const WARM_MICROPHONE_MS = 60_000;
const UPLOAD_TIMEOUT_MS = 30_000;
const MAX_PENDING_SEGMENTS = 3;
const VOICE_STOP_EVENT = "magichandy:emergency-stop";

type CaptureMode = "hands_free" | "hold";

export interface VoiceInputPreferences {
  input_mode: CaptureMode | string;
  input_sensitivity: number;
  input_silence_ms: number;
  input_noise_suppression: boolean;
}

interface VoiceComposerControlsProps {
  disabled: boolean;
  ready: boolean;
  unavailableTitle: string;
  preferences: VoiceInputPreferences;
  stopSequence?: number;
  onActivityChange: (active: boolean) => void;
  onTranscript: (transcript: string, stopSequence?: number) => Promise<void>;
  showError: (message: string) => void;
}

interface AudioInput {
  deviceId: string;
  label: string;
}

interface PendingSegment {
  samples: Float32Array;
  sampleRate: number;
  session: number;
  stopSequence?: number;
}

export function VoiceComposerControls({
  disabled,
  ready,
  unavailableTitle,
  preferences,
  stopSequence,
  onActivityChange,
  onTranscript,
  showError,
}: VoiceComposerControlsProps) {
  const [mode, setMode] = useState<CaptureMode>(normalizeMode(preferences.input_mode));
  const [sensitivity, setSensitivity] = useState(preferences.input_sensitivity);
  const [silenceMillis, setSilenceMillis] = useState(preferences.input_silence_ms);
  const [noiseSuppression, setNoiseSuppression] = useState(preferences.input_noise_suppression);
  const [savingPreferences, setSavingPreferences] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [selectedDevice, setSelectedDevice] = useState("default");
  const [audioInputs, setAudioInputs] = useState<AudioInput[]>([]);
  const [arming, setArming] = useState(false);
  const [listening, setListening] = useState(false);
  const [speechDetected, setSpeechDetected] = useState(false);
  const [recording, setRecording] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [microphoneReady, setMicrophoneReady] = useState(false);
  const [recordingSecondsLeft, setRecordingSecondsLeft] = useState(HOLD_RECORDING_LIMIT_SECONDS);
  const [inputLevel, setInputLevel] = useState(0);
  const [queuedSegments, setQueuedSegments] = useState(0);
  const mounted = useRef(true);
  const wantsCapture = useRef(false);
  const recorder = useRef<MediaRecorder | null>(null);
  const discardedRecorders = useRef(new WeakSet<MediaRecorder>());
  const microphoneStream = useRef<MediaStream | null>(null);
  const decoderContext = useRef<AudioContext | null>(null);
  const pcmStream = useRef<PCMStream | null>(null);
  const segmenter = useRef<VoiceActivitySegmenter | null>(null);
  const segmentQueue = useRef<PendingSegment[]>([]);
  const drainingSegments = useRef(false);
  const activeRequestID = useRef("");
  const activeRequestAbort = useRef<AbortController | null>(null);
  const captureSession = useRef(0);
  const handsFreeStopSequence = useRef<number | undefined>(undefined);
  const recordingTimer = useRef<number | null>(null);
  const countdownTimer = useRef<number | null>(null);
  const warmTimer = useRef<number | null>(null);
  const lastLevelUpdate = useRef(0);
  const lastStopSequence = useRef<number | undefined>(undefined);
  const stopSequenceRef = useRef(stopSequence);
  const selectedDeviceRef = useRef(selectedDevice);
  const onTranscriptRef = useRef(onTranscript);
  const deferredPreferences = useRef<VoiceInputPreferences | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  onTranscriptRef.current = onTranscript;
  stopSequenceRef.current = stopSequence;
  selectedDeviceRef.current = selectedDevice;
  const active = arming || listening || recording || processing;

  useEffect(() => onActivityChange(active), [active, onActivityChange]);

  useEffect(() => {
    const next = { ...preferences };
    if (active || wantsCapture.current) {
      deferredPreferences.current = next;
      return;
    }
    deferredPreferences.current = null;
    applyPreferences(next);
  }, [
    preferences.input_mode,
    preferences.input_sensitivity,
    preferences.input_silence_ms,
    preferences.input_noise_suppression,
  ]);

  useEffect(() => {
    if (active || wantsCapture.current || !deferredPreferences.current) return;
    const next = deferredPreferences.current;
    deferredPreferences.current = null;
    applyPreferences(next);
  }, [active]);

  function applyPreferences(next: VoiceInputPreferences) {
    setMode(normalizeMode(next.input_mode));
    setSensitivity(next.input_sensitivity);
    setSilenceMillis(next.input_silence_ms);
    setNoiseSuppression(next.input_noise_suppression);
  }

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
    if (!ready || disabled) abortCapture();
  }, [ready, disabled]);

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
      // Device labels can remain unavailable until microphone permission is granted.
    }
  }

  async function acquireMicrophone(session: number): Promise<MediaStream | null> {
    const current = microphoneStream.current;
    if (current?.getAudioTracks().some((track) => track.readyState === "live")) return current;
    if (!navigator.mediaDevices?.getUserMedia) {
      showError("Microphone capture requires localhost or HTTPS in a supported browser.");
      return null;
    }

    setArming(true);
    try {
      const audio: MediaTrackConstraints = {
        echoCancellation: true,
        autoGainControl: true,
        noiseSuppression,
        ...(selectedDevice === "default" ? {} : { deviceId: { exact: selectedDevice } }),
      };
      const stream = await navigator.mediaDevices.getUserMedia({ audio });
      if (!mounted.current || !wantsCapture.current || session !== captureSession.current) {
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

  async function startHandsFree() {
    if (disabled || !ready || active) return;
    wantsCapture.current = true;
    clearWarmTimer();
    const session = ++captureSession.current;
    const captureStopSequence = stopSequenceRef.current;
    handsFreeStopSequence.current = captureStopSequence;
    const stream = await acquireMicrophone(session);
    if (!stream || !mounted.current || session !== captureSession.current) {
      wantsCapture.current = false;
      if (session === captureSession.current) handsFreeStopSequence.current = undefined;
      return;
    }
    const buffered: Float32Array[] = [];
    let vad: VoiceActivitySegmenter | null = null;
    try {
      const pcm = await openPCMStream(stream, (samples) => {
        if (vad) handlePCMSamples(vad, samples, pcm.sampleRate, session, captureStopSequence);
        else buffered.push(samples);
      });
      if (!wantsCapture.current || session !== captureSession.current) {
        pcm.stop();
        releaseMicrophone();
        return;
      }
      vad = new VoiceActivitySegmenter({
        sampleRate: pcm.sampleRate,
        sensitivity,
        silenceMillis,
      });
      pcmStream.current = pcm;
      segmenter.current = vad;
      setListening(true);
      setSpeechDetected(false);
      for (const samples of buffered) handlePCMSamples(vad, samples, pcm.sampleRate, session, captureStopSequence);
    } catch (error) {
      wantsCapture.current = false;
      if (session === captureSession.current) handsFreeStopSequence.current = undefined;
      releaseMicrophone();
      if (mounted.current) showError(error instanceof Error ? error.message : "Continuous microphone capture could not start.");
    }
  }

  function handlePCMSamples(vad: VoiceActivitySegmenter, samples: Float32Array, sampleRate: number, session: number, captureStopSequence?: number) {
    if (session !== captureSession.current || !wantsCapture.current) return;
    const result = vad.push(samples);
    setSpeechDetected(result.speaking);
    const now = performance.now();
    if (now - lastLevelUpdate.current >= 80) {
      lastLevelUpdate.current = now;
      setInputLevel(result.level);
    }
    if (result.segment) enqueueSegment({ samples: result.segment, sampleRate, session, stopSequence: captureStopSequence });
  }

  function stopHandsFree(discard = false) {
    wantsCapture.current = false;
    const activeVAD = segmenter.current;
    const sampleRate = pcmStream.current?.sampleRate;
    const captureStopSequence = handsFreeStopSequence.current;
    handsFreeStopSequence.current = undefined;
    pcmStream.current?.stop();
    pcmStream.current = null;
    segmenter.current = null;
    setListening(false);
    setSpeechDetected(false);
    setInputLevel(0);
    releaseMicrophone();
    if (!discard && activeVAD && sampleRate) {
      const finalSegment = activeVAD.flush();
      if (finalSegment) enqueueSegment({
        samples: finalSegment,
        sampleRate,
        session: captureSession.current,
        stopSequence: captureStopSequence,
      });
    }
  }

  function enqueueSegment(next: PendingSegment) {
    if (next.session !== captureSession.current) return;
    if (segmentQueue.current.length >= MAX_PENDING_SEGMENTS) {
      showError("Voice transcription is falling behind; pause briefly before continuing.");
      return;
    }
    segmentQueue.current.push(next);
    setQueuedSegments(segmentQueue.current.length);
    setProcessing(true);
    void drainSegmentQueue();
  }

  async function drainSegmentQueue() {
    if (drainingSegments.current) return;
    drainingSegments.current = true;
    try {
      while (segmentQueue.current.length) {
        const next = segmentQueue.current.shift();
        setQueuedSegments(segmentQueue.current.length);
        if (!next || next.session !== captureSession.current) continue;
        try {
          const bytes = encodePCM16WAV([next.samples], next.sampleRate);
          const payload = new Uint8Array(bytes.byteLength);
          payload.set(bytes);
          const transcript = await transcribe(new Blob([payload], { type: "audio/wav" }), next.session, next.stopSequence);
          if (transcript && next.session === captureSession.current) {
            await onTranscriptRef.current(transcript, next.stopSequence);
          }
        } catch (error) {
          if (mounted.current && next.session === captureSession.current) {
            showError(error instanceof Error ? error.message : "Transcription failed.");
          }
        }
      }
    } finally {
      drainingSegments.current = false;
      if (mounted.current) setProcessing(false);
    }
  }

  async function startHoldRecording() {
    if (disabled || !ready || active || recorder.current) return;
    if (typeof MediaRecorder === "undefined") {
      showError("Hold-to-talk requires MediaRecorder support in this browser.");
      return;
    }
    wantsCapture.current = true;
    clearWarmTimer();
    const session = ++captureSession.current;
    const captureStopSequence = stopSequenceRef.current;
    const stream = await acquireMicrophone(session);
    if (!stream || !mounted.current || session !== captureSession.current) {
      wantsCapture.current = false;
      return;
    }

    try {
      const preferred = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus"]
        .find((mime) => MediaRecorder.isTypeSupported(mime));
      const nextRecorder = preferred ? new MediaRecorder(stream, { mimeType: preferred }) : new MediaRecorder(stream);
      const chunks: Blob[] = [];
      recorder.current = nextRecorder;
      nextRecorder.ondataavailable = (event) => { if (event.data.size > 0) chunks.push(event.data); };
      nextRecorder.onerror = () => {
        discardedRecorders.current.add(nextRecorder);
        showError("The browser could not record from the selected microphone.");
        stopHoldRecording(true);
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
        if (discardedRecorders.current.has(nextRecorder)) {
          if (mounted.current) setProcessing(false);
          return;
        }
        void finishHoldRecording(nextRecorder.mimeType || preferred || "audio/webm", chunks, session, captureStopSequence);
      };
      nextRecorder.start(100);
    } catch (error) {
      recorder.current = null;
      wantsCapture.current = false;
      releaseMicrophone();
      if (mounted.current) showError(error instanceof Error ? error.message : "The browser could not start microphone recording.");
    }
  }

  function stopHoldRecording(discard = false) {
    wantsCapture.current = false;
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

  async function finishHoldRecording(mimeType: string, chunks: Blob[], session: number, captureStopSequence?: number) {
    const blob = new Blob(chunks, { type: mimeType });
    if (blob.size === 0 || !mounted.current || session !== captureSession.current) {
      if (mounted.current) setProcessing(false);
      return;
    }
    try {
      const context = decoderContext.current ?? new AudioContext();
      decoderContext.current = context;
      const wav = await recordingToWAV(blob, context);
      const transcript = await transcribe(wav, session, captureStopSequence);
      if (transcript && mounted.current && session === captureSession.current) {
        await onTranscriptRef.current(transcript, captureStopSequence);
      } else if (mounted.current && session === captureSession.current) {
        showError("No speech was recognized.");
      }
    } catch (error) {
      if (mounted.current && session === captureSession.current) showError(error instanceof Error ? error.message : "Transcription failed.");
    } finally {
      if (mounted.current && session === captureSession.current) setProcessing(false);
      scheduleWarmRelease();
    }
  }

  async function transcribe(wav: Blob, session: number, captureStopSequence?: number): Promise<string> {
    const requestAbort = new AbortController();
    activeRequestAbort.current = requestAbort;
    let uploadTimedOut = false;
    const uploadTimer = window.setTimeout(() => {
      uploadTimedOut = true;
      requestAbort.abort();
    }, UPLOAD_TIMEOUT_MS);
    try {
      const submitted = await api.voiceTranscribe(wav, "wav", captureStopSequence, requestAbort.signal);
      window.clearTimeout(uploadTimer);
      activeRequestID.current = submitted.request.id;
      let transcriptionTimedOut = false;
      const transcriptionTimer = window.setTimeout(() => {
        transcriptionTimedOut = true;
        requestAbort.abort();
        void api.voiceRequestCancel(submitted.request.id).catch(() => undefined);
      }, 90_000);
      try {
        return await waitForTranscript(submitted.request.id, () => session !== captureSession.current, requestAbort.signal);
      } catch (error) {
        if (transcriptionTimedOut && session === captureSession.current) throw new Error("Transcription timed out.");
        throw error;
      } finally {
        window.clearTimeout(transcriptionTimer);
      }
    } catch (error) {
      if (uploadTimedOut && session === captureSession.current) throw new Error("Transcription upload timed out.");
      throw error;
    } finally {
      window.clearTimeout(uploadTimer);
      activeRequestID.current = "";
      if (activeRequestAbort.current === requestAbort) activeRequestAbort.current = null;
    }
  }

  function abortCapture() {
    captureSession.current++;
    wantsCapture.current = false;
    segmentQueue.current = [];
    handsFreeStopSequence.current = undefined;
    segmenter.current?.reset();
    segmenter.current = null;
    pcmStream.current?.stop();
    pcmStream.current = null;
    const requestID = activeRequestID.current;
    activeRequestID.current = "";
    activeRequestAbort.current?.abort();
    activeRequestAbort.current = null;
    if (requestID) void api.voiceRequestCancel(requestID).catch(() => undefined);
    stopHoldRecording(true);
    releaseMicrophone();
    if (mounted.current) {
      setQueuedSegments(0);
      setArming(false);
      setListening(false);
      setSpeechDetected(false);
      setRecording(false);
      setProcessing(false);
      setInputLevel(0);
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
    setRecordingSecondsLeft(HOLD_RECORDING_LIMIT_SECONDS);
    const startedAt = Date.now();
    countdownTimer.current = window.setInterval(() => {
      const left = HOLD_RECORDING_LIMIT_SECONDS - Math.floor((Date.now() - startedAt) / 1000);
      setRecordingSecondsLeft(Math.max(0, left));
    }, 1000);
    recordingTimer.current = window.setTimeout(() => stopHoldRecording(), HOLD_RECORDING_LIMIT_SECONDS * 1000);
  }

  function clearRecordingTimers() {
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    if (countdownTimer.current !== null) window.clearInterval(countdownTimer.current);
    recordingTimer.current = null;
    countdownTimer.current = null;
  }

  function scheduleWarmRelease() {
    if (warmTimer.current !== null || !microphoneStream.current || !mounted.current) return;
    warmTimer.current = window.setTimeout(() => {
      warmTimer.current = null;
      if (!recorder.current) releaseMicrophone();
    }, WARM_MICROPHONE_MS);
  }

  function clearWarmTimer() {
    if (warmTimer.current !== null) window.clearTimeout(warmTimer.current);
    warmTimer.current = null;
  }

  async function savePreference(patch: Partial<VoiceInputPreferences>) {
    setSavingPreferences(true);
    try {
      const saved = await api.saveVoiceInputPreferences(patch as Parameters<typeof api.saveVoiceInputPreferences>[0]);
      setMode(normalizeMode(saved.input_mode));
      setSensitivity(saved.input_sensitivity);
      setSilenceMillis(saved.input_silence_ms);
      setNoiseSuppression(saved.input_noise_suppression);
    } catch (error) {
      setMode(normalizeMode(preferences.input_mode));
      setSensitivity(preferences.input_sensitivity);
      setSilenceMillis(preferences.input_silence_ms);
      setNoiseSuppression(preferences.input_noise_suppression);
      showError(error instanceof Error ? error.message : "Voice input preference could not be saved.");
    } finally {
      setSavingPreferences(false);
    }
  }

  function selectInput(deviceId: string) {
    if (active || deviceId === selectedDevice) return;
    releaseMicrophone();
    setSelectedDevice(deviceId);
  }

  const label = arming
    ? "Cancel microphone startup"
    : listening
      ? "Stop hands-free listening"
      : recording
        ? `Stop and transcribe, ${recordingSecondsLeft} seconds remaining`
        : processing
          ? "Finishing voice transcription"
          : mode === "hands_free"
            ? "Start hands-free listening"
            : "Hold to talk";
  const title = ready ? label : unavailableTitle;

  return (
    <div className="voice-input" ref={menuRef} data-open={menuOpen || undefined}>
      <div className="voice-input-control">
        <button
          type="button"
          className="voice-mic-button"
          data-active={listening || recording || undefined}
          data-state={arming ? "arming" : processing ? "processing" : microphoneReady ? "ready" : undefined}
          disabled={disabled || !ready || (processing && !listening)}
          aria-label={label}
          aria-pressed={listening || recording}
          title={title}
          onClick={mode === "hands_free" ? () => arming ? abortCapture() : listening ? stopHandsFree() : void startHandsFree() : undefined}
          onPointerDown={mode === "hold" ? (event) => {
            event.preventDefault();
            event.currentTarget.setPointerCapture(event.pointerId);
            void startHoldRecording();
          } : undefined}
          onPointerUp={mode === "hold" ? (event) => { event.preventDefault(); stopHoldRecording(); } : undefined}
          onPointerCancel={mode === "hold" ? () => stopHoldRecording() : undefined}
          onKeyDown={mode === "hold" ? (event) => {
            if ((event.key === " " || event.key === "Enter") && !event.repeat) {
              event.preventDefault();
              void startHoldRecording();
            }
          } : undefined}
          onKeyUp={mode === "hold" ? (event) => {
            if (event.key === " " || event.key === "Enter") {
              event.preventDefault();
              stopHoldRecording();
            }
          } : undefined}
        >
          <MicrophoneIcon size={20} />
          {recording && <span className="voice-mic-countdown" aria-hidden="true">{recordingSecondsLeft}</span>}
          {active && <span className="voice-capture-label" aria-hidden="true">
            {arming ? "Starting microphone" : listening ? (speechDetected ? "Speech detected" : "Listening continuously") : processing ? "Transcribing" : "Listening"}
          </span>}
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
          <button type="button" disabled={savingPreferences} aria-pressed={mode === "hold"} onClick={() => { setMode("hold"); void savePreference({ input_mode: "hold" }); }}>Hold to talk</button>
          <button type="button" disabled={savingPreferences} aria-pressed={mode === "hands_free"} onClick={() => { setMode("hands_free"); void savePreference({ input_mode: "hands_free" }); }}>Hands-free</button>
        </div>
        <label className="field-label" htmlFor="voice-input-device">Microphone</label>
        <select id="voice-input-device" value={selectedDevice} disabled={active} onChange={(event) => selectInput(event.target.value)}>
          <option value="default">Default microphone</option>
          {audioInputs.filter((input) => input.deviceId && input.deviceId !== "default").map((input) => (
            <option key={input.deviceId} value={input.deviceId}>{input.label}</option>
          ))}
        </select>
        <label className="voice-range-label" htmlFor="voice-input-sensitivity">
          <span>Sensitivity</span><output>{sensitivity}</output>
        </label>
        <input
          id="voice-input-sensitivity"
          type="range"
          min={1}
          max={100}
          value={sensitivity}
          disabled={active || savingPreferences}
          onChange={(event) => setSensitivity(Number(event.target.value))}
          onPointerUp={() => void savePreference({ input_sensitivity: sensitivity })}
          onKeyUp={() => void savePreference({ input_sensitivity: sensitivity })}
        />
        <label className="voice-range-label" htmlFor="voice-input-silence">
          <span>End-of-speech delay</span><output>{(silenceMillis / 1000).toFixed(1)} s</output>
        </label>
        <input
          id="voice-input-silence"
          type="range"
          min={300}
          max={3000}
          step={100}
          value={silenceMillis}
          disabled={active || savingPreferences}
          onChange={(event) => setSilenceMillis(Number(event.target.value))}
          onPointerUp={() => void savePreference({ input_silence_ms: silenceMillis })}
          onKeyUp={() => void savePreference({ input_silence_ms: silenceMillis })}
        />
        <label className="voice-menu-toggle">
          <input
            type="checkbox"
            checked={noiseSuppression}
            disabled={active || savingPreferences}
            onChange={(event) => {
              setNoiseSuppression(event.target.checked);
              void savePreference({ input_noise_suppression: event.target.checked });
            }}
          />
          <span>Noise suppression</span>
        </label>
        <div className="voice-level" aria-label={`Microphone level ${Math.round(inputLevel * 100)} percent`}>
          <span style={{ width: `${Math.round(inputLevel * 100)}%` }} />
        </div>
        {queuedSegments > 0 && <span className="voice-queue-status" role="status">{queuedSegments} phrase{queuedSegments === 1 ? "" : "s"} queued</span>}
        {microphoneReady && <button type="button" className="voice-release-button" disabled={active} onClick={abortCapture}>Release microphone</button>}
      </div>
    </div>
  );
}

function normalizeMode(mode: string): CaptureMode {
  return mode === "hold" ? "hold" : "hands_free";
}

async function waitForTranscript(requestID: string, canceled: () => boolean, signal: AbortSignal): Promise<string> {
  for (;;) {
    if (canceled() || signal.aborted) {
      await api.voiceRequestCancel(requestID).catch(() => undefined);
      return "";
    }
    const res = await api.voiceRequest(requestID, signal);
    const request = res.request;
    if (request.role !== "asr" || request.type !== "transcribe") throw new Error("The voice worker returned the wrong request.");
    if (request.state === "done") return request.transcript?.[0]?.text?.trim() ?? "";
    if (request.state === "failed") throw new Error(request.error?.message || "Transcription failed.");
    if (request.state === "canceled") return "";
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
}
