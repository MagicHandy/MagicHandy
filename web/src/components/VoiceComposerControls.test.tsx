import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { VoiceComposerControls } from "./VoiceComposerControls";

vi.mock("../api/client", () => ({
  api: {
    voiceTranscribe: vi.fn(),
    voiceRequest: vi.fn(),
    voiceRequestCancel: vi.fn(),
  },
}));

vi.mock("../util/recording", () => ({
  recordingToWAV: vi.fn(async () => new Blob(["wav"], { type: "audio/wav" })),
}));

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

describe("VoiceComposerControls", () => {
  let track: FakeTrack;
  let getUserMedia: ReturnType<typeof vi.fn>;
  let enumerateDevices: ReturnType<typeof vi.fn>;

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
    voiceTranscribe.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "queued", created_at: "now" } });
    voiceRequest.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "done", created_at: "now", transcript: [{ text: "hello", confidence: 1 }] } });
    voiceRequestCancel.mockResolvedValue({ request: { id: "asr-3", role: "asr", type: "transcribe", state: "canceled", created_at: "now" } });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  function renderControls(onTranscript = vi.fn(async () => undefined)) {
    const result = render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        onActivityChange={() => undefined}
        onTranscript={onTranscript}
        showError={() => undefined}
      />,
    );
    return { ...result, onTranscript };
  }

  it("keeps the microphone warm between hands-free recordings", async () => {
    const { onTranscript } = renderControls();

    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    await screen.findByRole("button", { name: /stop and transcribe/i });
    fireEvent.click(screen.getByRole("button", { name: /stop and transcribe/i }));
    await waitFor(() => expect(onTranscript).toHaveBeenCalledWith("hello", undefined));

    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    await screen.findByRole("button", { name: /stop and transcribe/i });
    expect(getUserMedia).toHaveBeenCalledTimes(1);
  });

  it("offers hold-to-talk and sends the selected input constraint", async () => {
    renderControls();
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));
    const input = await screen.findByRole("combobox", { name: "Voice input" });
    fireEvent.change(input, { target: { value: "usb-mic" } });
    fireEvent.click(screen.getByRole("button", { name: "Hold to talk" }));
    fireEvent.click(screen.getByRole("button", { name: /close voice input menu/i }));

    const mic = screen.getByRole("button", { name: "Hold to talk" });
    fireEvent.keyDown(mic, { key: "Enter" });
    await screen.findByRole("button", { name: /stop and transcribe/i });
    fireEvent.keyUp(mic, { key: "Enter" });

    expect(getUserMedia).toHaveBeenCalledWith({ audio: { deviceId: { exact: "usb-mic" } } });
    await waitFor(() => expect(voiceTranscribe).toHaveBeenCalled());
    await screen.findByRole("button", { name: "Hold to talk" });
  });

  it("discards active capture immediately on Emergency Stop", async () => {
    const { onTranscript } = renderControls();
    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    await screen.findByRole("button", { name: /stop and transcribe/i });

    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(onTranscript).not.toHaveBeenCalled();
    expect(voiceTranscribe).not.toHaveBeenCalled();
  });

  it("rejects a microphone stream that resolves after startup is canceled", async () => {
    let resolveStream!: (stream: FakeStream) => void;
    getUserMedia.mockReturnValue(new Promise<FakeStream>((resolve) => { resolveStream = resolve; }));
    renderControls();

    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    fireEvent.click(await screen.findByRole("button", { name: /cancel microphone startup/i }));
    await act(async () => resolveStream(new FakeStream(track)));

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(FakeMediaRecorder.instances).toHaveLength(0);
    expect(voiceTranscribe).not.toHaveBeenCalled();
  });

  it("falls back to the default input when a selected device disappears", async () => {
    renderControls();
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));
    const input = await screen.findByRole("combobox", { name: "Voice input" });
    fireEvent.change(input, { target: { value: "usb-mic" } });
    expect(input).toHaveValue("usb-mic");

    enumerateDevices.mockResolvedValue([]);
    fireEvent.click(screen.getByRole("button", { name: /close voice input menu/i }));
    fireEvent.click(screen.getByRole("button", { name: /open voice input menu/i }));

    await waitFor(() => expect(screen.getByRole("combobox", { name: "Voice input" })).toHaveValue("default"));
  });

  it("clears parent activity when removed during capture", async () => {
    const onActivityChange = vi.fn();
    const result = render(
      <VoiceComposerControls
        disabled={false}
        ready
        unavailableTitle="Unavailable"
        onActivityChange={onActivityChange}
        onTranscript={async () => undefined}
        showError={() => undefined}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    await screen.findByRole("button", { name: /stop and transcribe/i });
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
      onActivityChange: () => undefined,
      onTranscript: async () => undefined,
      showError: () => undefined,
    };
    const result = render(<VoiceComposerControls {...props} stopSequence={4} />);
    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));
    await screen.findByRole("button", { name: /stop and transcribe/i });

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
        onActivityChange={() => undefined}
        onTranscript={async () => undefined}
        showError={showError}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /start hands-free voice/i }));

    await waitFor(() => expect(track.stop).toHaveBeenCalled());
    expect(showError).toHaveBeenCalledWith("recorder startup failed");
  });
});
