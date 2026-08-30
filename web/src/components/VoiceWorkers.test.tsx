import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { VoiceWorkerStatus } from "../api/types";
import { VoiceWorkers } from "./VoiceWorkers";

vi.mock("../api/client", () => ({
  api: {},
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: vi.fn() }),
}));

vi.mock("../state/voice-playback", () => ({
  useVoicePlayback: () => ({ queueSpeech: vi.fn() }),
}));

describe("VoiceWorkers", () => {
  it("omits a redundant disabled card when no provider is selected", () => {
    const { container } = render(
      <VoiceWorkers
        locked={false}
        role="asr"
        providerSelected={false}
        workers={{}}
        requests={[]}
        modules={{}}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("shows one actionable module readout when a managed install needs repair", () => {
    const repair = vi.fn(async () => undefined);
    render(
      <VoiceWorkers
        locked={false}
        role="asr"
        providerSelected
        showParakeetModule
        workers={{ asr: { role: "asr", state: "not_configured", configured: false, worker_queue_depth: 0, queue_depth: 0 } }}
        requests={[]}
        modules={{ parakeet: { state: "incomplete", installed: false, worker_installed: true, runtime_installed: false, runner_installed: false, model_installed: false, message: "Parakeet needs repair. Missing: runner, model." } }}
        parakeetRepair={{ setupBusy: false, error: "", repair, cancel: vi.fn(async () => undefined) }}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByRole("status", { name: "MagicHandy Parakeet module" })).toHaveTextContent("Missing: runner, model");
    fireEvent.click(screen.getByRole("button", { name: "Repair Parakeet" }));
    expect(repair).toHaveBeenCalledOnce();
    expect(screen.queryByText("The selected module is not ready; follow the module status above before starting it.")).not.toBeInTheDocument();
  });

  it("shows resumable byte progress and cancellation during repair", () => {
    const cancel = vi.fn(async () => undefined);
    render(
      <VoiceWorkers
        locked={false}
        role="asr"
        providerSelected
        showParakeetModule
        workers={{}}
        requests={[]}
        modules={{ parakeet: { state: "incomplete", installed: false, worker_installed: true, runtime_installed: false, resumable_partial: true, partial_bytes: 25, message: "Parakeet needs repair." } }}
        parakeetRepair={{
          setupBusy: true,
          error: "",
          job: { id: "repair", kind: "parakeet", module: "parakeet", device: "cpu", status: "running", message: "Parakeet model: 25 MiB of 100 MiB.", bytes_completed: 25, bytes_total: 100, started_at: "now", updated_at: "now" },
          repair: vi.fn(async () => undefined),
          cancel,
        }}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByRole("progressbar", { name: "Installation progress" })).toHaveAttribute("value", "25");
    expect(screen.getByRole("button", { name: "Repair Parakeet" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("keeps managed TTS recovery inside the app", () => {
    render(
      <VoiceWorkers
        locked={false}
        role="tts"
        providerSelected
        showTTSModule
        ttsModuleName="Faster Qwen3-TTS"
        workers={{}}
        requests={[]}
        modules={{ tts: { state: "missing", installed: false, worker_installed: false, runtime_installed: false, message: "Faster Qwen3-TTS is not installed." } }}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByRole("status", { name: "Checking the Faster Qwen3-TTS module." })).toHaveTextContent("Faster Qwen3-TTS is not installed.");
    expect(screen.getByRole("link", { name: "Run setup again" })).toHaveAttribute("href", "#/setup/reconfigure");
    expect(screen.queryByText(/powershell|\.ps1/i)).not.toBeInTheDocument();
  });

  it("shows an owned model-server failure while the adapter remains running", () => {
    const tts: VoiceWorkerStatus = {
      role: "tts",
      state: "running",
      configured: true,
      provider: "openai-compatible-tts",
      model_state: "",
      worker_queue_depth: 0,
      queue_depth: 0,
      last_error: "managed TTS server exited unexpectedly",
    };

    const { container } = render(
      <VoiceWorkers
        locked={false}
        role="tts"
        workers={{ tts }}
        requests={[]}
        modules={{}}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByText("Not ready")).toBeInTheDocument();
    expect(screen.getByText("managed TTS server exited unexpectedly")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Load model" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Send test" })).not.toBeInTheDocument();
    expect(container.querySelector('.status-dot[data-state="error"]')).not.toBeNull();
  });
});
