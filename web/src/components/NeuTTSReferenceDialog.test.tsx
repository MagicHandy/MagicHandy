import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { NeuTTSReferenceDialog } from "./NeuTTSReferenceDialog";

vi.mock("../api/client", () => ({
  clientId: "dialog-test",
  api: {
    pickHostPath: vi.fn(),
    prepareNeuTTSReference: vi.fn(),
  },
}));

describe("NeuTTSReferenceDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.prepareNeuTTSReference).mockResolvedValue({
      reference: {
        id: "abcdef0123456789abcdef01",
        codes_path: "C:\\MagicHandy\\references\\dave.npy",
        audio_path: "C:\\MagicHandy\\references\\dave.wav",
        transcript: "We are testing the reference voice.",
        token_count: 372,
        source_format: "torch_int32",
        reused: false,
      },
      preview_url: "/api/voice/neutts/references/abcdef0123456789abcdef01/audio",
    });
  });

  it("previews a prepared reference and applies the exact transcript", async () => {
    const onApply = vi.fn();
    const onClose = vi.fn();
    render(<NeuTTSReferenceDialog
      initialCodes="C:\\samples\\dave.pt"
      initialWAV=""
      initialTranscript=""
      onApply={onApply}
      onClose={onClose}
    />);

    fireEvent.click(screen.getByRole("button", { name: /prepare preview/i }));
    expect(await screen.findByText(/372 tokens validated/i)).toBeInTheDocument();
    expect(screen.getByText(/transcription guide/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/reference audio preview/i)).toHaveAttribute(
      "src",
      "/api/voice/neutts/references/abcdef0123456789abcdef01/audio?client_id=dialog-test",
    );
    expect(screen.getByLabelText(/exact reference transcript/i)).toHaveValue("We are testing the reference voice.");
    fireEvent.click(screen.getByRole("button", { name: /use reference/i }));
    expect(onApply).toHaveBeenCalledWith({
      codes: "C:\\MagicHandy\\references\\dave.npy",
      wav: "C:\\MagicHandy\\references\\dave.wav",
      transcript: "We are testing the reference voice.",
    });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("requires preview audio before applying a reference", async () => {
    vi.mocked(api.prepareNeuTTSReference).mockResolvedValueOnce({
      reference: {
        id: "abcdef0123456789abcdef01",
        codes_path: "C:\\MagicHandy\\references\\voice.npy",
        token_count: 20,
        source_format: "npy_int32",
        transcript: "Exact words.",
        reused: false,
      },
      preview_url: "",
    });
    render(<NeuTTSReferenceDialog
      initialCodes="C:\\samples\\voice.npy"
      initialWAV=""
      initialTranscript=""
      onApply={vi.fn()}
      onClose={vi.fn()}
    />);
    fireEvent.click(screen.getByRole("button", { name: /prepare preview/i }));
    expect(await screen.findByText(/no matching wav was found/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: /use reference/i })).toBeDisabled());
  });

  it("does not carry a previous transcript into a newly selected reference", async () => {
    render(<NeuTTSReferenceDialog
      initialCodes="C:\\samples\\old.npy"
      initialWAV="C:\\samples\\old.wav"
      initialTranscript="Words from the old clip."
      onApply={vi.fn()}
      onClose={vi.fn()}
    />);

    fireEvent.change(screen.getByRole("textbox", { name: /reference code source/i }), {
      target: { value: "C:\\samples\\dave.pt" },
    });
    expect(screen.queryByDisplayValue("Words from the old clip.")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /prepare preview/i }));
    expect(await screen.findByLabelText(/exact reference transcript/i)).toHaveValue("We are testing the reference voice.");
  });
});
