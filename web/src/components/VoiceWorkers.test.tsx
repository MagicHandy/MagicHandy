import { act, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { VoiceWorkers } from "./VoiceWorkers";

const mocks = vi.hoisted(() => ({
  show: vi.fn(),
  queueSpeech: vi.fn(),
  api: {
    voiceWorkerStart: vi.fn(),
    voiceWorkerStop: vi.fn(),
    voiceWorkerRestart: vi.fn(),
    voiceWorkerModel: vi.fn(),
    voiceWorkerTest: vi.fn(),
  },
}));

vi.mock("../api/client", () => ({ api: mocks.api }));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: mocks.show }),
}));

vi.mock("../state/voice-playback", () => ({
  useVoicePlayback: () => ({ queueSpeech: mocks.queueSpeech }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

function worker(overrides: Partial<VoiceWorkerStatus>): VoiceWorkerStatus {
  return { role: "tts", state: "running", configured: true, worker_queue_depth: 0, queue_depth: 0, ...overrides };
}

type Props = ComponentProps<typeof VoiceWorkers>;

function renderWorkers(props: Partial<Props> & Pick<Props, "role">) {
  const refresh = props.refresh ?? vi.fn(async () => undefined);
  const view = render(<VoiceWorkers locked={false} providerSelected workers={{}} requests={[]} modules={{}} {...props} refresh={refresh} />);
  return { ...view, refresh };
}

const status = (role: "tts" | "asr") => screen.getByRole("status", { name: role === "tts" ? "Speech output status" : "Speech input status" });

describe("VoiceWorkers", () => {
  beforeEach(() => {
    mocks.show.mockReset();
    mocks.queueSpeech.mockReset();
    Object.values(mocks.api).forEach((fn) => fn.mockReset());
  });

  it("omits the status when no provider is selected", () => {
    const { container } = renderWorkers({ role: "asr", providerSelected: false });
    expect(container).toBeEmptyDOMElement();
  });

  it("says Checking before the first snapshot and Unavailable when it never arrives", () => {
    const view = renderWorkers({ role: "asr", loaded: false });
    expect(status("asr")).toHaveTextContent("Checking…");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();

    view.rerender(<VoiceWorkers role="asr" locked={false} providerSelected loaded={false} statusError="MagicHandy could not be reached." workers={{}} requests={[]} modules={{}} refresh={view.refresh} />);
    expect(status("asr")).toHaveTextContent("Unavailable");
  });

  it("explains how to turn voice on", () => {
    renderWorkers({ role: "asr", enabled: false, workers: { asr: worker({ role: "asr", state: "disabled" }) } });
    expect(status("asr")).toHaveTextContent("Off");
    expect(status("asr")).toHaveTextContent("Turn on Enable voice above, then save.");
  });

  it("shows one actionable readout when a managed install needs repair", () => {
    const repair = vi.fn(async () => undefined);
    renderWorkers({
      role: "asr",
      showParakeetModule: true,
      workers: { asr: worker({ role: "asr", state: "not_configured", configured: false }) },
      modules: { parakeet: { state: "incomplete", installed: false, worker_installed: true, runtime_installed: false, runner_installed: false, model_installed: false, message: "Parakeet needs repair. Missing: runner, model." } },
      parakeetRepair: { setupBusy: false, error: "", repair, cancel: vi.fn(async () => undefined) },
    });

    expect(status("asr")).toHaveTextContent("Needs setup");
    expect(status("asr")).toHaveTextContent("Missing: runner, model");
    fireEvent.click(screen.getByRole("button", { name: "Repair Parakeet" }));
    expect(repair).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Start" })).not.toBeInTheDocument();
  });

  it("shows resumable byte progress and cancellation during repair", () => {
    const cancel = vi.fn(async () => undefined);
    renderWorkers({
      role: "asr",
      showParakeetModule: true,
      modules: { parakeet: { state: "incomplete", installed: false, worker_installed: true, runtime_installed: false, resumable_partial: true, partial_bytes: 25, message: "Parakeet needs repair." } },
      parakeetRepair: {
        setupBusy: true,
        error: "",
        job: { id: "repair", kind: "parakeet", module: "parakeet", device: "cpu", status: "running", message: "Parakeet model: 25 MiB of 100 MiB.", bytes_completed: 25, bytes_total: 100, started_at: "now", updated_at: "now" },
        repair: vi.fn(async () => undefined),
        cancel,
      },
    });

    expect(status("asr")).toHaveTextContent("Installing…");
    expect(status("asr")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("Parakeet model: 25 MiB of 100 MiB.")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "Installation progress" })).toHaveAttribute("value", "25");
    expect(screen.queryByRole("button", { name: "Repair Parakeet" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("offers a report for a failed Parakeet repair even when runtime actions are locked", () => {
    renderWorkers({
      role: "asr",
      locked: true,
      showParakeetModule: true,
      modules: { parakeet: { state: "incomplete", installed: false, worker_installed: true, runtime_installed: false, message: "Repair failed." } },
      parakeetRepair: { setupBusy: false, error: "", job: { id: "repair-failed", kind: "parakeet", module: "parakeet", device: "cpu", status: "failed", message: "Download failed.", started_at: "", updated_at: "" }, repair: vi.fn(), cancel: vi.fn() },
    });
    expect(status("asr")).toHaveTextContent("Setup failed");
    expect(status("asr")).toHaveTextContent("Download failed.");
    expect(screen.getByRole("button", { name: "Download failure report" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Repair Parakeet" })).toBeDisabled();
  });

  it("keeps Repair for an installed module in details and still reports its failure", () => {
    const { container } = renderWorkers({
      role: "asr",
      showParakeetModule: true,
      workers: { asr: worker({ role: "asr", model_state: "ready" }) },
      modules: { parakeet: { state: "ready", installed: true, worker_installed: true, runtime_installed: true, message: "MagicHandy's Parakeet worker, runner, and model are installed." } },
      parakeetRepair: { setupBusy: false, error: "parakeet setup is already running", repair: vi.fn(), cancel: vi.fn() },
    });
    expect(status("asr")).toHaveTextContent("Ready");
    expect(screen.getByRole("alert")).toHaveTextContent("parakeet setup is already running");
    const details = container.querySelector("details.voice-details") as HTMLElement;
    expect(within(details).getByText(/worker, runner, and model are installed/)).toBeInTheDocument();
    expect(within(details).getByRole("button", { name: "Repair Parakeet" })).toBeEnabled();
  });

  it("keeps managed TTS recovery inside the app", () => {
    renderWorkers({
      role: "tts",
      showTTSModule: true,
      modules: { tts: { state: "missing", installed: false, worker_installed: false, runtime_installed: false, message: "Faster Qwen3-TTS is not installed." } },
    });

    expect(status("tts")).toHaveTextContent("Faster Qwen3-TTS is not installed.");
    expect(screen.getByRole("link", { name: "Run setup again" })).toHaveAttribute("href", "#/setup/reconfigure");
    expect(screen.queryByText(/powershell|\.ps1/i)).not.toBeInTheDocument();
  });

  it("does not leave for guided setup while the form has unsaved changes", () => {
    renderWorkers({
      role: "tts",
      dirty: true,
      showTTSModule: true,
      modules: { tts: { state: "missing", installed: false, worker_installed: false, runtime_installed: false, message: "Faster Qwen3-TTS is not installed." } },
    });
    expect(screen.queryByRole("link", { name: "Run setup again" })).not.toBeInTheDocument();
  });

  it("shows Loading, not a failure, while the backend tracks the load", () => {
    renderWorkers({ role: "tts", workers: { tts: worker({ model_state: "loading" }) } });

    expect(status("tts")).toHaveTextContent("Loading…");
    expect(status("tts")).toHaveTextContent("This can take a minute or two");
    expect(status("tts")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByText("Failed")).not.toBeInTheDocument();
    for (const name of ["Play sample", "Load model", "Unload model", "Try again", "Start"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
  });

  it("says Busy while a running worker has not reported its model", () => {
    renderWorkers({ role: "asr", workers: { asr: worker({ role: "asr", model_state: "" }) } });
    expect(status("asr")).toHaveTextContent("Busy");
    expect(status("asr")).toHaveTextContent("normal while the model loads");
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop" })).toBeEnabled();
  });

  it("keeps an error reported beside loading visible as a failure", () => {
    const { container } = renderWorkers({ role: "tts", workers: { tts: worker({ model_state: "loading", last_error: "TTS server did not become ready: port in use" }) } });
    expect(status("tts")).toHaveTextContent("Failed");
    expect(container.querySelector(".voice-status-error")).toHaveTextContent("port in use");
  });

  it("restarts a running worker that stopped answering, with plain wording and the raw error in details", async () => {
    mocks.api.voiceWorkerRestart.mockResolvedValue({ worker: worker({ role: "asr" }) });
    const { container } = renderWorkers({ role: "asr", workers: { asr: worker({ role: "asr", model_state: "", last_error: "context deadline exceeded" }) } });

    expect(status("asr")).toHaveTextContent("Failed");
    expect(status("asr")).toHaveTextContent("It is running but not working.");
    expect(container.querySelector(".voice-status-error")).toHaveTextContent("It did not respond in time.");
    const details = container.querySelector("details.voice-details");
    expect(details).not.toHaveAttribute("open");
    expect(within(details as HTMLElement).getByText("context deadline exceeded")).toBeInTheDocument();

    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Try again" })); });
    expect(mocks.api.voiceWorkerRestart).toHaveBeenCalledWith("asr");
    expect(mocks.api.voiceWorkerModel).not.toHaveBeenCalled();
  });

  it("shows an owned model-server failure while the adapter remains running", async () => {
    mocks.api.voiceWorkerRestart.mockResolvedValue({ worker: worker({ model_state: "ready" }) });
    const { container, refresh } = renderWorkers({
      role: "tts",
      workers: { tts: worker({ provider: "openai-compatible-tts", model_state: "", last_error: "managed TTS server exited unexpectedly" }) },
    });

    expect(status("tts")).toHaveTextContent("Failed");
    expect(status("tts")).toHaveTextContent("It is running but not working.");
    expect(container.querySelector(".voice-status-error")).toHaveTextContent("managed TTS server exited unexpectedly");
    expect(container.querySelector('.status-dot[data-state="error"]')).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Play sample" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop" })).toBeEnabled();

    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Try again" })); });
    expect(mocks.api.voiceWorkerRestart).toHaveBeenCalledWith("tts");
    expect(refresh).toHaveBeenCalled();
  });

  it("restarts a crashed worker and keeps its output in technical details", async () => {
    mocks.api.voiceWorkerRestart.mockResolvedValue({ worker: worker({}) });
    const tail = "Traceback: CUDA out of memory";
    const { container } = renderWorkers({
      role: "tts",
      workers: { tts: worker({ state: "crashed", last_error: `voice worker exited unexpectedly: exit status 1 — stderr: ${tail}`, stderr_tail: tail }) },
    });

    expect(container.querySelector(".voice-status-error")).toHaveTextContent("voice worker exited unexpectedly: exit status 1");
    expect(container.querySelector(".voice-status-error")).not.toHaveTextContent("CUDA");
    expect(container.querySelector(".voice-details-output")).toHaveTextContent(tail);

    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Try again" })); });
    expect(mocks.api.voiceWorkerRestart).toHaveBeenCalledWith("tts");
  });

  it("keeps long startup output in collapsed details with a concise recovery action", () => {
    const error = `Loading failed\n${"CUDA allocation diagnostic. ".repeat(20)}`;
    const { container } = renderWorkers({ role: "tts", workers: { tts: worker({ last_error: error }) } });
    expect(status("tts")).toHaveTextContent("Failed");
    expect(container.querySelector(".voice-status-error")).toHaveTextContent("Voice ran into a problem. Try again, or open Technical details for the full error.");
    expect(container.querySelector(".voice-status-error")).not.toHaveTextContent("CUDA");
    const details = container.querySelector("details.voice-details");
    expect(details).not.toHaveAttribute("open");
    expect(details).toHaveTextContent("CUDA allocation diagnostic.");
    expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
  });

  it("labels a timed-out start after the worker stopped", () => {
    const { container } = renderWorkers({ role: "asr", workers: { asr: worker({ role: "asr", state: "stopped", last_error: "context deadline exceeded" }) } });
    expect(status("asr")).toHaveTextContent("It could not start.");
    expect(container.querySelector(".voice-status-error")).toHaveTextContent("It did not respond in time.");
  });

  it("shows this tab's start as loading and stays quiet when the user cancels it", async () => {
    const start = deferred<unknown>();
    const stop = deferred<unknown>();
    mocks.api.voiceWorkerStart.mockReturnValue(start.promise);
    mocks.api.voiceWorkerStop.mockReturnValue(stop.promise);
    const view = renderWorkers({ role: "tts", workers: { tts: worker({ state: "stopped" }) } });

    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(status("tts")).toHaveTextContent("Starting…");
    expect(mocks.api.voiceWorkerStart).toHaveBeenCalledWith("tts");

    view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={{ tts: worker({ model_state: "loading" }) }} requests={[]} modules={{}} refresh={view.refresh} />);
    expect(status("tts")).toHaveTextContent("Loading…");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mocks.api.voiceWorkerStop).toHaveBeenCalledWith("tts");
    expect(status("tts")).toHaveTextContent("Stopping…");

    await act(async () => {
      start.reject(new Error("voice worker could not become ready: context canceled"));
      stop.resolve({ worker: worker({ state: "stopped" }) });
    });
    expect(mocks.show).not.toHaveBeenCalled();

    view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={{ tts: worker({ state: "stopped" }) }} requests={[]} modules={{}} refresh={view.refresh} />);
    expect(status("tts")).toHaveTextContent("Stopped");
  });

  it("does not let a cancelled start's late answer clear or report against the next start", async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    mocks.api.voiceWorkerStart.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    mocks.api.voiceWorkerStop.mockResolvedValue({ worker: worker({ state: "stopped" }) });
    const stopped = { tts: worker({ state: "stopped" }) };
    const view = renderWorkers({ role: "tts", workers: stopped });
    const rerender = (workers: Record<string, VoiceWorkerStatus>) =>
      view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={workers} requests={[]} modules={{}} refresh={view.refresh} />);

    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    rerender({ tts: worker({ model_state: "loading" }) });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Cancel" })); });
    rerender(stopped);
    expect(status("tts")).toHaveTextContent("Stopped");

    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(mocks.api.voiceWorkerStart).toHaveBeenCalledTimes(2);
    expect(status("tts")).toHaveTextContent("Starting…");

    // The first start answers late, once as an error...
    await act(async () => { first.reject(new Error("voice worker could not become ready: context canceled")); });
    expect(mocks.show).not.toHaveBeenCalled();
    expect(status("tts")).toHaveTextContent("Starting…");
    expect(screen.queryByRole("button", { name: "Start" })).not.toBeInTheDocument();

    // ...and the second start's own failure is still reported.
    await act(async () => { second.reject(new Error("voice worker could not become ready: missing model")); });
    expect(mocks.show).toHaveBeenCalledOnce();
    expect(mocks.show).toHaveBeenCalledWith("Start failed: voice worker could not become ready: missing model", "error");
    expect(status("tts")).toHaveTextContent("Stopped");
  });

  it("does not let an obsolete start's late success end the newer start's pending state", async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    mocks.api.voiceWorkerStart.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    mocks.api.voiceWorkerStop.mockResolvedValue({});
    const stopped = { tts: worker({ state: "stopped" }) };
    const view = renderWorkers({ role: "tts", workers: stopped });

    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Cancel" })); });
    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await act(async () => { first.resolve({ worker: worker({ model_state: "ready" }) }); });
    expect(status("tts")).toHaveTextContent("Starting…");

    await act(async () => { second.resolve({ worker: worker({ model_state: "ready" }) }); });
    view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={{ tts: worker({ model_state: "ready" }) }} requests={[]} modules={{}} refresh={view.refresh} />);
    expect(status("tts")).toHaveTextContent("Ready");
  });

  it("surfaces a crash that happens while this tab's start is still waiting", () => {
    mocks.api.voiceWorkerStart.mockReturnValue(new Promise(() => undefined));
    const view = renderWorkers({ role: "tts", workers: { tts: worker({ state: "stopped" }) }, snapshotRequestedAt: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    const rerender = (workers: Record<string, VoiceWorkerStatus>, snapshotRequestedAt: number) =>
      view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={workers} requests={[]} modules={{}} refresh={view.refresh} snapshotRequestedAt={snapshotRequestedAt} />);
    const crashed = { tts: worker({ state: "crashed", last_error: "voice worker exited unexpectedly: exit status 3" }) };

    // A snapshot requested before the click cannot describe this start.
    rerender(crashed, 1);
    expect(status("tts")).toHaveTextContent("Starting…");

    rerender(crashed, Date.now() + 1);
    expect(status("tts")).toHaveTextContent("Failed");
    expect(view.container.querySelector(".voice-status-error")).toHaveTextContent("exit status 3");
    expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
  });

  it("keeps the start pending while a newer snapshot still shows the failure it is retrying", () => {
    mocks.api.voiceWorkerRestart.mockReturnValue(new Promise(() => undefined));
    const crashed = { tts: worker({ state: "crashed", last_error: "voice worker exited unexpectedly: exit status 1" }) };
    const view = renderWorkers({ role: "tts", workers: crashed, snapshotRequestedAt: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={crashed} requests={[]} modules={{}} refresh={view.refresh} snapshotRequestedAt={Date.now() + 1} />);
    expect(status("tts")).toHaveTextContent("Starting…");
    expect(view.container.querySelector(".voice-status-error")).toBeNull();
  });

  it("reports a failed start that the user did not cancel", async () => {
    mocks.api.voiceWorkerStart.mockRejectedValue(new Error("voice worker could not become ready: missing model"));
    renderWorkers({ role: "asr", workers: { asr: worker({ role: "asr", state: "stopped" }) } });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Start" })); });
    expect(mocks.show).toHaveBeenCalledWith("Start failed: voice worker could not become ready: missing model", "error");
  });

  it("plays a TTS sample and reports the result of that test only", async () => {
    const queued: VoiceRequestSnapshot = { id: "sample-1", role: "tts", type: "speak", state: "queued", created_at: "2026-10-26T10:00:00Z" };
    mocks.api.voiceWorkerTest.mockResolvedValue({ request: queued });
    const ready = { tts: worker({ model_state: "ready" }) };
    const older: VoiceRequestSnapshot = { id: "chat-0", role: "tts", type: "speak", state: "failed", created_at: "2026-10-26T09:00:00Z", error: { code: "timeout", message: "old failure" } };
    const view = renderWorkers({ role: "tts", workers: ready, requests: [older] });

    expect(status("tts")).toHaveTextContent("Ready");
    expect(view.container.querySelector('.status-dot[data-state="ok"]')).not.toBeNull();
    // An older chat request's failure stays in technical details, not the status.
    expect(view.container.querySelector(".voice-status-error")).toBeNull();
    expect(within(view.container.querySelector("details.voice-details") as HTMLElement).getByText(/old failure/)).toBeInTheDocument();

    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Play sample" })); });
    expect(mocks.api.voiceWorkerTest).toHaveBeenCalledWith("tts", expect.objectContaining({ delay_ms: 0 }));
    expect(mocks.queueSpeech).toHaveBeenCalledWith("sample-1");

    view.rerender(<VoiceWorkers role="tts" locked={false} providerSelected workers={ready} requests={[older, { ...queued, state: "done", completion_ms: 1240, audio_bytes: 4096 }]} modules={{}} refresh={view.refresh} />);
    expect(screen.getByText("Sample generated in 1.2 s.")).toBeInTheDocument();
  });

  it("drops a sample that answers after the user stopped the worker", async () => {
    const sample = deferred<{ request: VoiceRequestSnapshot }>();
    mocks.api.voiceWorkerTest.mockReturnValue(sample.promise);
    mocks.api.voiceWorkerStop.mockResolvedValue({});
    renderWorkers({ role: "tts", workers: { tts: worker({ model_state: "ready" }) } });

    fireEvent.click(screen.getByRole("button", { name: "Play sample" }));
    expect(screen.getByRole("button", { name: "Testing…" })).toBeDisabled();
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Stop" })); });
    await act(async () => { sample.resolve({ request: { id: "late", role: "tts", type: "speak", state: "queued", created_at: "2026-10-26T10:00:00Z" } }); });
    expect(mocks.queueSpeech).not.toHaveBeenCalled();
    expect(mocks.show).not.toHaveBeenCalled();
  });

  it("checks speech input with a silent clip", async () => {
    const request: VoiceRequestSnapshot = { id: "asr-1", role: "asr", type: "transcribe", state: "done", created_at: "2026-10-26T10:00:00Z", rejected: "no_speech" };
    mocks.api.voiceWorkerTest.mockResolvedValue({ request });
    renderWorkers({ role: "asr", workers: { asr: worker({ role: "asr", model_state: "ready" }) } });

    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Test input" })); });
    expect(mocks.queueSpeech).not.toHaveBeenCalled();
    expect(screen.getByText("Input test passed. The model processed a silent test clip.")).toBeInTheDocument();
  });

  it("locks start and tests behind unsaved changes but keeps Stop available", () => {
    renderWorkers({ role: "tts", dirty: true, workers: { tts: worker({ model_state: "ready" }) } });
    expect(screen.getByRole("button", { name: "Play sample" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Stop" })).toBeEnabled();
    expect(screen.getByText("Save settings to apply your changes before starting or testing.")).toBeInTheDocument();
  });

  it("asks to save a newly chosen provider before showing worker controls", () => {
    renderWorkers({ role: "asr", dirty: true, workers: { asr: worker({ role: "asr", state: "disabled" }) } });
    expect(status("asr")).toHaveTextContent("Not saved");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("waits for a current snapshot before acting on stale status", () => {
    renderWorkers({ role: "asr", statusError: "MagicHandy could not be reached.", workers: { asr: worker({ role: "asr", state: "stopped" }) } });
    expect(status("asr")).toHaveTextContent("Stopped");
    expect(screen.getByRole("button", { name: "Start" })).toBeDisabled();
  });

  it("keeps runtime actions disabled for a locked client", () => {
    renderWorkers({ role: "asr", locked: true, workers: { asr: worker({ role: "asr", model_state: "ready" }) } });
    expect(screen.getByRole("button", { name: "Test input" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Stop" })).toBeDisabled();
  });
});
