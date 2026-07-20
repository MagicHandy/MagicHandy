import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { NeuTTSReferenceDialog } from "./NeuTTSReferenceDialog";

vi.mock("../api/client", () => ({
  clientId: "dialog-test",
  api: {
    pickHostPath: vi.fn(),
    generateNeuTTSReference: vi.fn(),
  },
}));

describe("NeuTTSReferenceDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.generateNeuTTSReference).mockResolvedValue({
      reference: {
        id: "abcdef0123456789abcdef01",
        codes_path: "C:/MagicHandy/references/dave.npy",
        audio_path: "C:/MagicHandy/references/dave.wav",
        transcript: "We are testing the reference voice.",
        token_count: 373,
        source_format: "neucodec_onnx",
        reused: false,
      },
      preview_url: "/api/voice/neutts/references/abcdef0123456789abcdef01/audio",
    });
  });

  it("generates codes from only a WAV and transcript, previews, and applies them", async () => {
    const onApply = vi.fn();
    const onClose = vi.fn();
    render(<NeuTTSReferenceDialog
      initialWAV="C:/samples/dave.wav"
      initialTranscript="We are testing the reference voice."
      onApply={onApply}
      onClose={onClose}
    />);

    fireEvent.click(screen.getByRole("button", { name: /generate reference codes/i }));
    await waitFor(() => expect(api.generateNeuTTSReference).toHaveBeenCalledWith(
      "C:/samples/dave.wav",
      "We are testing the reference voice.",
      expect.any(AbortSignal),
    ));
    expect(await screen.findByText(/373 codes generated/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/reference audio preview/i)).toHaveAttribute(
      "src",
      "/api/voice/neutts/references/abcdef0123456789abcdef01/audio?client_id=dialog-test",
    );
    fireEvent.click(screen.getByRole("button", { name: /use reference/i }));
    expect(onApply).toHaveBeenCalledWith({
      codes: "C:/MagicHandy/references/dave.npy",
      wav: "C:/MagicHandy/references/dave.wav",
      transcript: "We are testing the reference voice.",
    });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("requires both source inputs before generation", () => {
    render(<>
      <button type="button" data-emergency-stop>Emergency stop all motion</button>
      <NeuTTSReferenceDialog
        initialWAV=""
        initialTranscript=""
        onApply={vi.fn()}
        onClose={vi.fn()}
      />
    </>);

    const generate = screen.getByRole("button", { name: /generate reference codes/i });
    expect(generate).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: /source voice/i }), {
      target: { value: "C:/samples/voice.wav" },
    });
    expect(generate).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: /exact source transcript/i }), {
      target: { value: "Exact words." },
    });
    expect(generate).toBeEnabled();

    screen.getByRole("button", { name: /close reference voice window/i }).focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(screen.getByRole("button", { name: /emergency stop all motion/i })).toHaveFocus();
  });

  it("applies a corrected transcript without re-encoding the same audio", async () => {
    const onApply = vi.fn();
    render(<NeuTTSReferenceDialog
      initialWAV="C:/samples/voice.wav"
      initialTranscript="Wrong word."
      onApply={onApply}
      onClose={vi.fn()}
    />);

    fireEvent.click(screen.getByRole("button", { name: /generate reference codes/i }));
    expect(await screen.findByText(/373 codes generated/i)).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: /exact source transcript/i }), {
      target: { value: "Correct word." },
    });
    fireEvent.click(screen.getByRole("button", { name: /use reference/i }));

    expect(api.generateNeuTTSReference).toHaveBeenCalledOnce();
    expect(onApply).toHaveBeenCalledWith(expect.objectContaining({ transcript: "Correct word." }));
  });

  it("invalidates generated codes when the source WAV changes", async () => {
    render(<NeuTTSReferenceDialog
      initialWAV="C:/samples/old.wav"
      initialTranscript="Exact words."
      onApply={vi.fn()}
      onClose={vi.fn()}
    />);

    fireEvent.click(screen.getByRole("button", { name: /generate reference codes/i }));
    expect(await screen.findByText(/373 codes generated/i)).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: /source voice/i }), {
      target: { value: "C:/samples/new.wav" },
    });

    expect(screen.queryByText(/373 codes generated/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /use reference/i })).toBeDisabled();
  });
});
