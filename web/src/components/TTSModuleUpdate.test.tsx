import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { SetupJob, VoiceModuleUpdate } from "../api/types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { TTSModuleUpdate } from "./TTSModuleUpdate";

vi.mock("../api/client", () => ({ api: { updateTTSModule: vi.fn(), cancelTTSModuleUpdate: vi.fn() } }));
const available: VoiceModuleUpdate = { available: true, supported: true, id: "bundled-version", module: "faster-qwen3-tts" };
const job: SetupJob = { id: "job-1", kind: "tts_update", module: "faster-qwen3-tts", device: "cuda", status: "running", message: "Preparing the runtime.", started_at: "", updated_at: "" };

describe("managed TTS updates", () => {
  beforeEach(() => {
    setLocaleForTest("en", english);
    vi.resetAllMocks();
    vi.mocked(api.updateTTSModule).mockResolvedValue({ installation: job });
    vi.mocked(api.cancelTTSModuleUpdate).mockResolvedValue({ installation: job });
  });

  it("waits for an explicit click and sends the displayed update identity", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    render(<TTSModuleUpdate update={available} locked={false} refresh={refresh} onComplete={vi.fn()} />);
    expect(screen.getByText(/several GiB/)).toBeVisible();
    expect(api.updateTTSModule).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Update TTS module" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(api.updateTTSModule).toHaveBeenCalledOnce();
    expect(api.updateTTSModule).toHaveBeenCalledWith("bundled-version");
  });

  it("honors controller, unsaved settings, platform and installer locks", () => {
    const props = { refresh: vi.fn(), onComplete: vi.fn() };
    const view = render(<TTSModuleUpdate {...props} update={available} locked />);
    expect(screen.getByRole("button", { name: "Update TTS module" })).toBeDisabled();
    view.rerender(<TTSModuleUpdate {...props} update={{ ...available, busy: true }} locked={false} />);
    expect(screen.getByRole("button", { name: "Update TTS module" })).toBeDisabled();
    view.rerender(<TTSModuleUpdate {...props} update={{ ...available, supported: false }} locked={false} />);
    expect(screen.getByRole("button", { name: "Update TTS module" })).toBeDisabled();
    expect(api.updateTTSModule).not.toHaveBeenCalled();
  });

  it("cancels the displayed job and permits retry after a failure", async () => {
    const props = { refresh: vi.fn().mockResolvedValue(undefined), onComplete: vi.fn(), locked: false };
    const view = render(<TTSModuleUpdate {...props} update={{ ...available, busy: true, job }} />);
    expect(screen.getByText(job.message)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(api.cancelTTSModuleUpdate).toHaveBeenCalledWith(job.id));
    expect(api.cancelTTSModuleUpdate).toHaveBeenCalledOnce();
    view.rerender(<TTSModuleUpdate {...props} update={{ ...available, job: { ...job, status: "failed", message: "Download failed." } }} />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Update TTS module" })).toBeEnabled());
    vi.mocked(api.updateTTSModule).mockRejectedValue(new Error("Request failed"));
    fireEvent.click(screen.getByRole("button", { name: "Update TTS module" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Request failed");
  });

  it("refreshes saved settings once after activation and hides a current idle module", async () => {
    const props = { refresh: vi.fn(), onComplete: vi.fn(), locked: false };
    const update = { ...available, available: false, job: { ...job, status: "complete" } };
    const view = render(<TTSModuleUpdate {...props} update={update} />);
    await waitFor(() => expect(props.onComplete).toHaveBeenCalledOnce());
    view.rerender(<TTSModuleUpdate {...props} update={{ ...update }} />);
    expect(props.onComplete).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Update TTS module" })).not.toBeInTheDocument();
    view.rerender(<TTSModuleUpdate {...props} update={{ ...available, available: false }} />);
    expect(view.container).toBeEmptyDOMElement();
  });
});
