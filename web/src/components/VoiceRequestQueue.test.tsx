import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { VoiceRequestSnapshot } from "../api/types";
import { VoiceRequestQueue } from "./VoiceRequestQueue";

const show = vi.hoisted(() => vi.fn());

vi.mock("../api/client", () => ({
  api: { voiceRequestCancel: vi.fn() },
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show }),
}));

const requests: VoiceRequestSnapshot[] = [
  { id: "tts-13", role: "tts", type: "speak", state: "active", created_at: "2026-07-15T12:00:00Z" },
  { id: "asr-14", role: "asr", type: "transcribe", state: "queued", created_at: "2026-07-15T12:00:01Z" },
  { id: "tts-12", role: "tts", type: "speak", state: "done", created_at: "2026-07-15T11:59:00Z", audio_bytes: 100 },
];

describe("VoiceRequestQueue", () => {
  beforeEach(() => {
    show.mockReset();
    vi.mocked(api.voiceRequestCancel).mockReset();
    vi.mocked(api.voiceRequestCancel).mockResolvedValue({ request: { ...requests[0], state: "canceled" } });
  });

  it("renders ASR and TTS work in one labeled queue", () => {
    render(<VoiceRequestQueue locked={false} requests={requests} refresh={vi.fn(async () => undefined)} />);

    const queue = screen.getByRole("region", { name: "Voice queue" });
    expect(within(queue).getByText("Speech output")).toBeInTheDocument();
    expect(within(queue).getByText("Speech input")).toBeInTheDocument();
    expect(within(queue).getAllByRole("button", { name: "Cancel" })).toHaveLength(2);
    expect(within(queue).queryByText("#tts-12")).not.toBeInTheDocument();
    expect(screen.getAllByRole("region", { name: "Voice queue" })).toHaveLength(1);
  });

  it("cancels the selected backend request and refreshes the shared snapshot", async () => {
    const refresh = vi.fn(async () => undefined);
    render(<VoiceRequestQueue locked={false} requests={requests} refresh={refresh} />);

    fireEvent.click(screen.getAllByRole("button", { name: "Cancel" })[0]);

    await waitFor(() => expect(api.voiceRequestCancel).toHaveBeenCalledWith("tts-13"));
    expect(refresh).toHaveBeenCalledOnce();
  });
});
