import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { openPCMStream } from "../util/voice-capture";
import { VoiceComposerControls } from "./VoiceComposerControls";

vi.mock("../api/client", () => ({
  api: {
    voiceTranscribe: vi.fn(),
    voiceRequest: vi.fn(),
    voiceRequestCancel: vi.fn(),
    saveVoiceInputPreferences: vi.fn(),
  },
}));

vi.mock("../util/recording", () => ({
  recordingToWAV: vi.fn(async () => new Blob(["wav"], { type: "audio/wav" })),
  encodePCM16WAV: vi.fn(() => new Uint8Array(48)),
}));

vi.mock("../util/voice-capture", () => ({ openPCMStream: vi.fn() }));

class FakeTrack extends EventTarget {
  readyState: MediaStreamTrackState = "live";
  stop = vi.fn(() => { this.readyState = "ended"; });
}

class FakeStream {
  constructor(readonly track: FakeTrack) {}
  getTracks = () => [this.track] as unknown as MediaStreamTrack[];
  getAudioTracks = () => [this.track] as unknown as MediaStreamTrack[];
}

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];
  static failConstruction = false;
  static isTypeSupported = () => true;
  state: RecordingState = "inactive";
  mimeType: string;
  ondataavailable: ((event: BlobEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onstart: ((event: Event) => void) | null = null;
  onstop: ((event: Event) => void) | null = null;

  constructor(_stream: MediaStream, options?: MediaRecorderOptions) {
    if (FakeMediaRecorder.failConstruction) throw new Error("recorder startup failed");
    this.mimeType = options?.mimeType ?? "audio/webm";
    FakeMediaRecorder.instances.push(this);
  }

  start() {
    this.state = "recording";
    this.onstart?.(new Event("start"));
  }

  stop() {
    this.state = "inactive";
    this.ondataavailable?.({ data: new Blob(["recording"], { type: this.mimeType }) } as BlobEvent);
    this.onstop?.(new Event("stop"));
  }
}

const voiceTranscribe = vi.mocked(api.voiceTranscribe);
const voiceRequest = vi.mocked(api.voiceRequest);
const voiceRequestCancel = vi.mocked(api.voiceRequestCancel);
const saveVoiceInputPreferences = vi.mocked(api.saveVoiceInputPreferences);
const openPCM = vi.mocked(openPCMStream);

const preferences = {
  input_mode: "hands_free" as const,
  input_sensitivity: 55,
  input_silence_ms: 300,
  input_noise_suppression: true,
};

describe("VoiceComposerControls", () => {
  let track: FakeTrack;
  let getUserMedia: ReturnType<typeof vi.fn>;
  let enumerateDevices: ReturnType<typeof vi.fn>;
  let emitPCM: (samples: Float32Array) => void;
  let stopPCM: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    FakeMediaRecorder.instances = [];
    FakeMediaRecorder.failConstruction = false;
    track = new FakeTrack();
    getUserMedia = vi.fn(async () => new FakeStream(track));
    enumerateDevices = vi.fn(async () => [
      { kind: "audioinput", deviceId: "usb-mic", label: "Studio microphone", groupId: "group", toJSON: () => ({}) },
    ]);
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: { getUserMedia, enumerateDevices, addEventListener: vi.fn(), removeEventListener: vi.fn() },
    });
    vi.stubGlobal("MediaRecorder", FakeMediaRecorder);
    vi.stubGlobal("AudioContext", class { close = vi.fn(async () => undefined); });
    stopPCM = vi.fn();
    openPCM.mockImplementation(async (_stream, onSamples) => {
      emitPCM = onSamples;
      return { sampleRate: 1000, stop: stopPCM };
    });
    voiceTranscribe.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "queued", created_at: "now" } });
    voiceRequest.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "done", created_at: "now", transcript: [{ text: "hello", confidence: 1 }] } });
    voiceRequestCancel.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "canceled", created_at: "now" } });
    saveVoiceInputPreferences.mockImplementation(async (patch) => ({ ...preferences, ...patch }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  function renderControls(onTranscript = vi.fn(async () => undefined), inputPreferences = preferences) {
    const result = render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        preferences={inputPreferences}
        onActivityChange={() => undefined}
        onTranscript={onTranscript}
        showError={() => undefined}
      />,
    );
    return { ...result, onTranscript };
  }

  it("keeps hands-free listening active across phrase transcriptions", async () => {
    const { onTranscript } = renderControls();

    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });
    act(() => {
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50).fill(0.2));
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50));
    });
    await waitFor(() => expect(onTranscript).toHaveBeenCalledWith("hello", undefined));
    expect(screen.getByRole("button", { name: /stop hands-free listening/i })).toBeInTheDocument();
    expect(track.stop).not.toHaveBeenCalled();

    act(() => {
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50).fill(0.2));
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50));
    });
    await waitFor(() => expect(onTranscript).toHaveBeenCalledTimes(2));
    expect(getUserMedia).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /stop hands-free listening/i }));
    expect(stopPCM).toHaveBeenCalled();
    expect(track.stop).toHaveBeenCalled();
  });

  it("keeps the capture-start Stop sequence when the final phrase is flushed", async () => {
    const props = {
      disabled: false,
      ready: true,
      unavailableTitle: "Unavailable",
      preferences,
      onActivityChange: () => undefined,
      onTranscript: async () => undefined,
      showError: () => undefined,
    };
    const result = render(<VoiceComposerControls {...props} stopSequence={4} />);
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });
    act(() => {
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50).fill(0.2));
    });

    result.rerender(<VoiceComposerControls {...props} stopSequence={undefined} />);
    fireEvent.click(screen.getByRole("button", { name: /stop hands-free listening/i }));

    await waitFor(() => expect(voiceTranscribe).toHaveBeenCalled());
    expect(voiceTranscribe).toHaveBeenCalledWith(expect.any(Blob), "wav", 4, expect.any(AbortSignal));
  });

  it("continues draining queued phrases after one transcription fails", async () => {
    let rejectFirst!: (reason: Error) => void;
    voiceTranscribe.mockImplementationOnce(() => new Promise((_, reject) => { rejectFirst = reject; }));
    const onTranscript = vi.fn(async () => undefined);
    const showError = vi.fn();
    render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        preferences={preferences}
        onActivityChange={() => undefined}
        onTranscript={onTranscript}
        showError={showError}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });
    const phrase = () => {
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50).fill(0.2));
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50));
    };
    act(phrase);
    await waitFor(() => expect(voiceTranscribe).toHaveBeenCalledTimes(1));
    act(phrase);
    await screen.findByText(/1 phrase queued/i);
    await act(async () => rejectFirst(new Error("first phrase failed")));

    await waitFor(() => expect(onTranscript).toHaveBeenCalledWith("hello", undefined));
    expect(showError).toHaveBeenCalledWith("first phrase failed");
    expect(screen.getByRole("button", { name: /stop hands-free listening/i })).toBeInTheDocument();
  });

  it("aborts an in-flight transcription poll on Emergency Stop", async () => {
    let pollSignal: AbortSignal | undefined;
    voiceRequest.mockImplementation((_id, signal) => new Promise((_resolve, reject) => {
      pollSignal = signal;
      signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
    }));
    const showError = vi.fn();
    render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        preferences={preferences}
        onActivityChange={() => undefined}
        onTranscript={async () => undefined}
        showError={showError}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });
    act(() => {
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50).fill(0.2));
      for (let index = 0; index < 6; index += 1) emitPCM(new Float32Array(50));
    });
    await waitFor(() => expect(voiceRequest).toHaveBeenCalled());

    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));

    await waitFor(() => expect(pollSignal?.aborted).toBe(true));
    expect(voiceRequestCancel).toHaveBeenCalledWith("asr-3");
    expect(track.stop).toHaveBeenCalled();
    expect(showError).not.toHaveBeenCalled();
  });

  it("persists input mode, sensitivity, delay, and noise suppression", async () => {
    renderControls();
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));
    fireEvent.click(screen.getByRole("button", { name: "Hold to talk" }));
    await waitFor(() => expect(saveVoiceInputPreferences).toHaveBeenCalledWith({ input_mode: "hold" }));

    saveVoiceInputPreferences.mockClear();
    const sensitivity = screen.getByRole("slider", { name: /sensitivity/i });
    fireEvent.change(sensitivity, { target: { value: "72" } });
    fireEvent.pointerUp(sensitivity);
    fireEvent.blur(sensitivity);
    await waitFor(() => expect(saveVoiceInputPreferences).toHaveBeenCalledWith({ input_sensitivity: 72 }));
    expect(saveVoiceInputPreferences).toHaveBeenCalledTimes(1);

    saveVoiceInputPreferences.mockClear();
    const delay = screen.getByRole("slider", { name: /end-of-speech delay/i });
    fireEvent.change(delay, { target: { value: "1300" } });
    fireEvent.pointerUp(delay);
    await waitFor(() => expect(saveVoiceInputPreferences).toHaveBeenCalledWith({ input_silence_ms: 1300 }));

    saveVoiceInputPreferences.mockClear();
    fireEvent.click(screen.getByRole("checkbox", { name: /noise suppression/i }));
    await waitFor(() => expect(saveVoiceInputPreferences).toHaveBeenCalledWith({ input_noise_suppression: false }));
  });

  it("offers hold-to-talk and sends the selected input constraint", async () => {
    renderControls();
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));
    const input = await screen.findByRole("combobox", { name: "Microphone" });
    fireEvent.change(input, { target: { value: "usb-mic" } });
    fireEvent.click(screen.getByRole("button", { name: "Hold to talk" }));
    fireEvent.click(screen.getByRole("button", { name: /close voice input menu/i }));

    const mic = screen.getByRole("button", { name: "Hold to talk" });
    fireEvent.keyDown(mic, { key: "Enter" });
    await screen.findByRole("button", { name: /stop and transcribe/i });
    fireEvent.keyUp(mic, { key: "Enter" });

    expect(getUserMedia).toHaveBeenCalledWith({ audio: {
      autoGainControl: true,
      deviceId: { exact: "usb-mic" },
      echoCancellation: true,
      noiseSuppression: true,
    } });
    await waitFor(() => expect(voiceTranscribe).toHaveBeenCalled());
    await screen.findByRole("button", { name: "Hold to talk" });
  });

  it("discards active capture immediately on Emergency Stop", async () => {
    const { onTranscript } = renderControls();
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });

    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(stopPCM).toHaveBeenCalled();
    expect(onTranscript).not.toHaveBeenCalled();
    expect(voiceTranscribe).not.toHaveBeenCalled();
  });

  it("rejects a microphone stream that resolves after startup is canceled", async () => {
    let resolveStream!: (stream: FakeStream) => void;
    getUserMedia.mockReturnValue(new Promise<FakeStream>((resolve) => { resolveStream = resolve; }));
    renderControls();

    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    fireEvent.click(await screen.findByRole("button", { name: /cancel microphone startup/i }));
    await act(async () => resolveStream(new FakeStream(track)));

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(FakeMediaRecorder.instances).toHaveLength(0);
    expect(voiceTranscribe).not.toHaveBeenCalled();
  });

  it("falls back to the default input when a selected device disappears", async () => {
    renderControls();
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));
    const input = await screen.findByRole("combobox", { name: "Microphone" });
    fireEvent.change(input, { target: { value: "usb-mic" } });
    expect(input).toHaveValue("usb-mic");

    enumerateDevices.mockResolvedValue([]);
    fireEvent.click(screen.getByRole("button", { name: /close voice input menu/i }));
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));

    await waitFor(() => expect(screen.getByRole("combobox", { name: "Microphone" })).toHaveValue("default"));
  });

  it("clears parent activity when removed during capture", async () => {
    const onActivityChange = vi.fn();
    const result = render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        preferences={preferences}
        onActivityChange={onActivityChange}
        onTranscript={async () => undefined}
        showError={() => undefined}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });
    await waitFor(() => expect(onActivityChange).toHaveBeenCalledWith(true));

    result.unmount();

    expect(onActivityChange).toHaveBeenLastCalledWith(false);
    expect(track.stop).toHaveBeenCalled();
  });

  it("aborts capture when another client advances the Stop sequence", async () => {
    const props = {
      disabled: false,
      ready: true,
      unavailableTitle: "Unavailable",
      preferences,
      onActivityChange: () => undefined,
      onTranscript: async () => undefined,
      showError: () => undefined,
    };
    const result = render(<VoiceComposerControls {...props} stopSequence={4} />);
    fireEvent.click(screen.getByRole("button", { name: /start hands-free listening/i }));
    await screen.findByRole("button", { name: /stop hands-free listening/i });

    result.rerender(<VoiceComposerControls {...props} stopSequence={5} />);

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(voiceTranscribe).not.toHaveBeenCalled();
  });

  it("releases the stream when recorder construction fails", async () => {
    const showError = vi.fn();
    FakeMediaRecorder.failConstruction = true;
    render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        preferences={{ ...preferences, input_mode: "hold" }}
        onActivityChange={() => undefined}
        onTranscript={async () => undefined}
        showError={showError}
      />,
    );

    const mic = screen.getByRole("button", { name: /hold to talk/i });
    fireEvent.keyDown(mic, { key: "Enter" });

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(showError).toHaveBeenCalledWith("recorder startup failed");
  });
});
