import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ChatPanel } from "./ChatPanel";

const app = vi.hoisted(() => ({
  state: {
    uptime_seconds: 1,
    stop_sequence: 1,
    chat: { latest_seq: 0 },
    settings: { voice: { enabled: false, asr_provider: "none" } },
  },
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    getChatMessages: vi.fn(),
    advanceChatCursor: vi.fn(),
  },
  streamChat: vi.fn(),
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    backendOnline: true,
    readOnly: false,
    state: app.state,
  }),
  useToast: () => ({ show: app.show }),
}));

vi.mock("../state/voice-playback", () => ({
  useVoicePlayback: () => ({ queueSpeech: vi.fn() }),
}));

const getChatMessages = vi.mocked(api.getChatMessages);
const advanceChatCursor = vi.mocked(api.advanceChatCursor);

describe("ChatPanel history", () => {
  beforeEach(() => {
    app.state = {
      uptime_seconds: 1,
      stop_sequence: 1,
      chat: { latest_seq: 0 },
      settings: { voice: { enabled: false, asr_provider: "none" } },
    };
    app.show.mockReset();
    getChatMessages.mockReset();
    advanceChatCursor.mockReset();
    advanceChatCursor.mockResolvedValue({ cursor: 0 });
  });

  it("distinguishes a failed history read from an empty conversation and retries", async () => {
    getChatMessages
      .mockRejectedValueOnce(new Error("chat database unavailable"))
      .mockResolvedValueOnce({
        messages: [{ seq: 1, role: "assistant", content: "Recovered conversation", created_at: "now" }],
        latest_seq: 1,
        cursor: 0,
      });

    render(<ChatPanel />);

    expect(await screen.findByRole("alert")).toHaveTextContent("chat database unavailable");
    expect(screen.queryByText("No messages yet")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Message" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Recovered conversation")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled();
  });

  it("retries a transient tail failure on the next state poll", async () => {
    getChatMessages
      .mockResolvedValueOnce({
        messages: [{ seq: 1, role: "user", content: "First", created_at: "now" }],
        latest_seq: 1,
        cursor: 1,
      })
      .mockRejectedValueOnce(new Error("temporary read failure"))
      .mockResolvedValueOnce({
        messages: [{ seq: 2, role: "assistant", content: "Second", created_at: "now" }],
        latest_seq: 2,
        cursor: 1,
      });
    app.state = { ...app.state, chat: { latest_seq: 1 } };
    const result = render(<ChatPanel />);
    expect(await screen.findByText("First")).toBeInTheDocument();

    app.state = { ...app.state, uptime_seconds: 2, chat: { latest_seq: 2 } };
    result.rerender(<ChatPanel />);
    expect(await screen.findByText(/Conversation updates delayed/)).toHaveTextContent("temporary read failure");
    expect(getChatMessages).toHaveBeenCalledTimes(2);

    app.state = { ...app.state, uptime_seconds: 3 };
    result.rerender(<ChatPanel />);

    expect(await screen.findByText("Second")).toBeInTheDocument();
    await waitFor(() => expect(getChatMessages).toHaveBeenCalledTimes(3));
  });
});
