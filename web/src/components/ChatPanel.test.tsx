import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ChatPanel } from "./ChatPanel";

const app = vi.hoisted(() => ({
  sessionId: "chat-test",
  state: {
    uptime_seconds: 1,
    stop_sequence: 1,
    chat: { latest_seq: 0, active_session_id: "chat-test" },
    settings: { voice: { enabled: false, asr_provider: "none" } },
  },
  show: vi.fn(),
  queueSpeech: vi.fn(),
}));
const SESSION_ID = app.sessionId;

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
  useVoicePlayback: () => ({ queueSpeech: app.queueSpeech }),
}));

const getChatMessages = vi.mocked(api.getChatMessages);
const advanceChatCursor = vi.mocked(api.advanceChatCursor);

describe("ChatPanel history", () => {
  beforeEach(() => {
    app.state = {
      uptime_seconds: 1,
      stop_sequence: 1,
      chat: { latest_seq: 0, active_session_id: SESSION_ID },
      settings: { voice: { enabled: false, asr_provider: "none" } },
    };
    app.show.mockReset();
    app.queueSpeech.mockReset();
    getChatMessages.mockReset();
    advanceChatCursor.mockReset();
    advanceChatCursor.mockResolvedValue({ cursor: 0, session_id: SESSION_ID });
  });

  it("distinguishes a failed history read from an empty conversation and retries", async () => {
    getChatMessages
      .mockRejectedValueOnce(new Error("chat database unavailable"))
      .mockResolvedValueOnce({
        messages: [{ seq: 1, role: "assistant", content: "Recovered conversation", created_at: "now" }],
        latest_seq: 1,
        cursor: 0,
        session_id: SESSION_ID,
      });

    render(<ChatPanel sessionId={SESSION_ID} />);

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
        session_id: SESSION_ID,
      })
      .mockRejectedValueOnce(new Error("temporary read failure"))
      .mockResolvedValueOnce({
        messages: [{ seq: 2, role: "assistant", content: "Second", created_at: "now" }],
        latest_seq: 2,
        cursor: 1,
        session_id: SESSION_ID,
      });
    app.state = { ...app.state, chat: { latest_seq: 1, active_session_id: SESSION_ID } };
    const result = render(<ChatPanel sessionId={SESSION_ID} />);
    expect(await screen.findByText("First")).toBeInTheDocument();

    app.state = { ...app.state, uptime_seconds: 2, chat: { latest_seq: 2, active_session_id: SESSION_ID } };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);
    expect(await screen.findByText(/Conversation updates delayed/)).toHaveTextContent("temporary read failure");
    expect(getChatMessages).toHaveBeenCalledTimes(2);

    app.state = { ...app.state, uptime_seconds: 3 };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);

    expect(await screen.findByText("Second")).toBeInTheDocument();
    await waitFor(() => expect(getChatMessages).toHaveBeenCalledTimes(3));
  });

  it("plays new autonomous replies but does not replay speech from initial history", async () => {
    getChatMessages
      .mockResolvedValueOnce({
        messages: [{
          seq: 1,
          role: "assistant",
          content: "Earlier line",
          created_at: "now",
          speech_request_id: "tts-old",
        }],
        latest_seq: 1,
        cursor: 1,
        session_id: SESSION_ID,
      })
      .mockResolvedValueOnce({
        messages: [{
          seq: 2,
          role: "assistant",
          content: "New autonomous line",
          created_at: "now",
          speech_request_id: "tts-new",
        }],
        latest_seq: 2,
        cursor: 1,
        session_id: SESSION_ID,
      });
    app.state = { ...app.state, chat: { latest_seq: 1, active_session_id: SESSION_ID } };
    const result = render(<ChatPanel sessionId={SESSION_ID} />);
    expect(await screen.findByText("Earlier line")).toBeInTheDocument();
    expect(app.queueSpeech).not.toHaveBeenCalled();

    app.state = { ...app.state, uptime_seconds: 2, chat: { latest_seq: 2, active_session_id: SESSION_ID } };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);

    expect(await screen.findByText("New autonomous line")).toBeInTheDocument();
    expect(app.queueSpeech).toHaveBeenCalledWith("tts-new");
  });

  it("exposes persisted model-run diagnostics from the assistant avatar", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [{
        seq: 1,
        role: "assistant",
        content: "Diagnosed reply",
        created_at: "now",
        diagnostics: {
          source: "interactive",
          provider: "llama_cpp",
          model: "gemma-3",
          prompt_set: "magichandy_motion_v1",
          request_ms: 184,
          motion_action: "target",
        },
      }],
      latest_seq: 1,
      cursor: 1,
      session_id: SESSION_ID,
    });

    render(<ChatPanel sessionId={SESSION_ID} />);

    expect(await screen.findByText("Diagnosed reply")).toBeInTheDocument();
    const avatar = screen.getByRole("button", { name: "Show response diagnostics" });
    expect(avatar).toHaveAttribute("title", expect.stringContaining("Model: gemma-3"));
    expect(screen.getByRole("tooltip")).toHaveTextContent(/Run time\s*184 ms/);
  });
});
