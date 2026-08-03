import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api, streamChat } from "../api/client";
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
  refresh: vi.fn(),
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
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

vi.mock("../state/voice-playback", () => ({
  useVoicePlayback: () => ({ queueSpeech: app.queueSpeech }),
}));

const getChatMessages = vi.mocked(api.getChatMessages);
const advanceChatCursor = vi.mocked(api.advanceChatCursor);
const streamChatMock = vi.mocked(streamChat);

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
    app.refresh.mockReset();
    app.refresh.mockResolvedValue(undefined);
    getChatMessages.mockReset();
    advanceChatCursor.mockReset();
    streamChatMock.mockReset();
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
          preparation_ms: 12,
          scheduler_wait_ms: 31,
          first_token_ms: 96,
          generation_ms: 141,
          repair_ms: 22,
          provider_calls: 2,
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
    expect(screen.getByRole("tooltip")).toHaveTextContent(/First token\s*96 ms/);
    expect(screen.getByRole("tooltip")).toHaveTextContent(/Model queue\s*31 ms/);
    expect(screen.getByRole("tooltip")).toHaveTextContent(/Repair\s*22 ms/);
    expect(screen.getByRole("tooltip")).toHaveTextContent(/Provider calls\s*2/);
  });

  it("marks persona changes from persisted assistant provenance", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [
        {
          seq: 1,
          role: "assistant",
          content: "Rowan line",
          created_at: "now",
          diagnostics: {
            source: "interactive",
            persona_id: "persona-rowan",
            persona_name: "Rowan",
          },
        },
        { seq: 2, role: "user", content: "Switch", created_at: "now" },
        {
          seq: 3,
          role: "assistant",
          content: "Mara line",
          created_at: "now",
          diagnostics: {
            source: "interactive",
            persona_id: "persona-mara",
            persona_name: "Mara",
          },
        },
        {
          seq: 4,
          role: "assistant",
          content: "Still Mara",
          created_at: "now",
          diagnostics: {
            source: "autopilot",
            persona_id: "persona-mara",
            persona_name: "Mara",
          },
        },
      ],
      latest_seq: 4,
      cursor: 4,
      session_id: SESSION_ID,
    });

    render(<ChatPanel sessionId={SESSION_ID} />);

    expect(await screen.findByText("Mara line")).toBeInTheDocument();
    expect(screen.getAllByRole("separator")).toHaveLength(1);
    expect(screen.getByRole("separator")).toHaveTextContent("Persona changed to Mara");
    expect(screen.queryByText("Persona changed to Rowan")).not.toBeInTheDocument();
    expect(screen.getAllByText("Mara").length).toBeGreaterThanOrEqual(2);
  });

  it("refreshes authoritative state before completing a chat turn", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockImplementation(async (_sessionId, _message, onEvent) => {
      onEvent({ event: "status", data: { state: "deterministic_stop", user_seq: 1, stop_sequence: 0 } });
      onEvent({ event: "message", data: { reply: "Stopped.", seq: 2 } });
      onEvent({ event: "done", data: { ok: true } });
    });
    let finishRefresh: (() => void) | undefined;
    app.refresh.mockReturnValue(new Promise<void>((resolve) => { finishRefresh = resolve; }));
    const onSessionChanged = vi.fn();
    render(<ChatPanel sessionId={SESSION_ID} onSessionChanged={onSessionChanged} />);

    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "please stop" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(app.refresh).toHaveBeenCalledTimes(1));
    expect(onSessionChanged).not.toHaveBeenCalled();
    finishRefresh?.();
    await waitFor(() => expect(onSessionChanged).toHaveBeenCalledTimes(1));
  });

  it("keeps the initial reply stable while a repair streams", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    let emit: Parameters<typeof streamChat>[2] | undefined;
    let finish: (() => void) | undefined;
    streamChatMock.mockImplementation((_sessionId, _message, onEvent) => {
      emit = onEvent;
      return new Promise<void>((resolve) => { finish = resolve; });
    });

    const onBusyChange = vi.fn();
    render(<ChatPanel sessionId={SESSION_ID} onBusyChange={onBusyChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "say something" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(emit).toBeDefined());

    act(() => emit?.({ event: "delta", data: { text: "{\"reply\":\"First draft\"" } }));
    expect(await screen.findByText("First draft")).toBeInTheDocument();

    act(() => {
      emit?.({ event: "malformed", data: {} });
      emit?.({ event: "repair_delta", data: { text: "{\"reply\":\"Replacement still streaming" } });
      emit?.({ event: "delta", data: { phase: "repair", text: " more" } });
    });
    expect(screen.getByText("First draft")).toBeInTheDocument();
    expect(screen.queryByText("Replacement still streaming")).not.toBeInTheDocument();

    await act(async () => {
      emit?.({ event: "message", data: { reply: "Repaired reply", seq: 2, initial_malformed: true } });
      emit?.({ event: "done", data: { ok: true } });
      finish?.();
      await Promise.resolve();
    });
    expect(await screen.findByText("Repaired reply")).toBeInTheDocument();
    expect(screen.queryByText("First draft")).not.toBeInTheDocument();
    expect(onBusyChange).toHaveBeenLastCalledWith(false);
  });

  it("replaces rejected drafts when the repair also stays malformed", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockImplementation(async (_sessionId, _message, onEvent) => {
      onEvent({ event: "delta", data: { text: "{\"reply\":\"Rejected first draft" } });
      onEvent({ event: "malformed", data: { recoverable: true } });
      onEvent({ event: "repair_delta", data: { text: "{\"reply\":\"Rejected repair" } });
      onEvent({ event: "malformed", data: { recoverable: false } });
      onEvent({ event: "done", data: { ok: false, malformed: true } });
    });

    render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "say something" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Malformed model response.")).toBeInTheDocument();
    expect(screen.queryByText("Rejected first draft")).not.toBeInTheDocument();
    expect(screen.queryByText("Rejected repair")).not.toBeInTheDocument();
  });

  it("sends with Enter while preserving Shift+Enter and IME composition", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockResolvedValue(undefined);

    render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "first line" } });

    fireEvent.keyDown(textbox, { key: "Enter", shiftKey: true });
    fireEvent.keyDown(textbox, { key: "Enter", isComposing: true });
    expect(streamChatMock).not.toHaveBeenCalled();

    fireEvent.change(textbox, { target: { value: "first line\nsecond line" } });
    fireEvent.keyDown(textbox, { key: "Enter" });

    await waitFor(() => expect(streamChatMock).toHaveBeenCalledWith(
      SESSION_ID,
      "first line\nsecond line",
      expect.any(Function),
      expect.any(AbortSignal),
      1,
    ));
  });

  it("surfaces a deterministic Stop transport failure", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockImplementation(async (_sessionId, _message, onEvent) => {
      onEvent({ event: "status", data: { state: "deterministic_stop", stop_sequence: 2 } });
      onEvent({ event: "message", data: { reply: "Stopping motion.", seq: 2 } });
      onEvent({ event: "motion", data: { applied: true, action: "stop", error: "transport timed out" } });
      onEvent({ event: "done", data: { ok: false } });
    });

    render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "stop" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(app.show).toHaveBeenCalledWith(
      "Device Stop could not be confirmed: transport timed out",
      "error",
    ));
    expect(await screen.findByText("Stopping motion.")).toBeInTheDocument();
  });

  it("keeps a committed reply visible when Stop cancels only its motion and speech", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockImplementation(async (_sessionId, _message, onEvent) => {
      onEvent({ event: "message", data: { reply: "Easing into it.", seq: 4 } });
      onEvent({ event: "error", data: {
        message: "Emergency Stop canceled this reply's motion and speech.",
        reply_retained: "true",
      } });
      onEvent({ event: "done", data: { ok: false } });
    });

    render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "start slow" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(app.show).toHaveBeenCalledWith(
      "Emergency Stop canceled this reply's motion and speech.",
      "error",
    ));
    // The reply is already in canonical history; replacing it with the error
    // would contradict what a reload shows.
    expect(await screen.findByText("Easing into it.")).toBeInTheDocument();
    expect(screen.queryByText("Emergency Stop canceled this reply's motion and speech.")).not.toBeInTheDocument();
  });

  it("still replaces the bubble when the whole turn is canceled before a reply", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    streamChatMock.mockImplementation(async (_sessionId, _message, onEvent) => {
      onEvent({ event: "error", data: { message: "Chat canceled by Emergency Stop." } });
      onEvent({ event: "done", data: { ok: false } });
    });

    render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "start slow" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Chat canceled by Emergency Stop.")).toBeInTheDocument();
  });

  it("aborts the active stream without callbacks when the panel unmounts", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    let streamSignal: AbortSignal | undefined;
    streamChatMock.mockImplementation((_sessionId, _message, onEvent, signal) => {
      streamSignal = signal;
      onEvent({ event: "delta", data: { text: "{\"reply\":\"Uncommitted draft" } });
      onEvent({ event: "malformed", data: { recoverable: true } });
      onEvent({ event: "repair_delta", data: { text: "{\"reply\":\"Uncommitted repair" } });
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        );
      });
    });
    const onBusyChange = vi.fn();
    const result = render(<ChatPanel sessionId={SESSION_ID} onBusyChange={onBusyChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "keep talking" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(streamChatMock).toHaveBeenCalledOnce());
    expect(await screen.findByText("Uncommitted draft")).toBeInTheDocument();

    result.unmount();
    expect(streamSignal?.aborted).toBe(true);
    await Promise.resolve();
    await Promise.resolve();

    expect(app.show).not.toHaveBeenCalled();
    expect(app.queueSpeech).not.toHaveBeenCalled();
    expect(app.refresh).not.toHaveBeenCalled();
    expect(onBusyChange).toHaveBeenLastCalledWith(false);
  });

  it("aborts the active stream and unlocks composition after Emergency Stop", async () => {
    getChatMessages.mockResolvedValueOnce({
      messages: [],
      latest_seq: 0,
      cursor: 0,
      session_id: SESSION_ID,
    });
    let streamSignal: AbortSignal | undefined;
    streamChatMock.mockImplementation((_sessionId, _message, onEvent, signal) => {
      streamSignal = signal;
      onEvent({ event: "delta", data: { text: "{\"reply\":\"Uncommitted draft" } });
      onEvent({ event: "malformed", data: { recoverable: true } });
      onEvent({ event: "repair_delta", data: { text: "{\"reply\":\"Uncommitted repair" } });
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        );
      });
    });
    const result = render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "keep talking" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(streamChatMock).toHaveBeenCalledOnce());
    expect(await screen.findByText("Uncommitted draft")).toBeInTheDocument();

    app.state = { ...app.state, stop_sequence: 2 };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);

    await waitFor(() => expect(streamSignal?.aborted).toBe(true));
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled());
    expect(screen.queryByText("Uncommitted draft")).not.toBeInTheDocument();
    expect(app.show).not.toHaveBeenCalled();
    expect(app.queueSpeech).not.toHaveBeenCalled();
    expect(app.refresh).not.toHaveBeenCalled();
  });

  it("removes a staged reply when Emergency Stop races its commit", async () => {
    getChatMessages
      .mockResolvedValueOnce({
        messages: [],
        latest_seq: 0,
        cursor: 0,
        session_id: SESSION_ID,
      })
      .mockResolvedValueOnce({
        messages: [{ seq: 1, role: "user", content: "keep talking", created_at: "now" }],
        latest_seq: 1,
        cursor: 0,
        session_id: SESSION_ID,
      });
    let streamSignal: AbortSignal | undefined;
    streamChatMock.mockImplementation((_sessionId, _message, onEvent, signal) => {
      streamSignal = signal;
      onEvent({ event: "message", data: { reply: "Staged but not committed", seq: 2 } });
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        );
      });
    });
    const result = render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "keep talking" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("Staged but not committed")).toBeInTheDocument();

    app.state = { ...app.state, stop_sequence: 2 };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);

    await waitFor(() => expect(streamSignal?.aborted).toBe(true));
    await waitFor(() => expect(screen.queryByText("Staged but not committed")).not.toBeInTheDocument());
    expect(getChatMessages).toHaveBeenCalledTimes(2);
  });

  it("restores a committed reply when Stop wins the retention-event race", async () => {
    getChatMessages
      .mockResolvedValueOnce({
        messages: [],
        latest_seq: 0,
        cursor: 0,
        session_id: SESSION_ID,
      })
      .mockResolvedValueOnce({
        messages: [
          { seq: 1, role: "user", content: "keep talking", created_at: "now" },
          { seq: 2, role: "assistant", content: "Committed before Stop", created_at: "now" },
        ],
        latest_seq: 2,
        cursor: 0,
        session_id: SESSION_ID,
      });
    let streamSignal: AbortSignal | undefined;
    streamChatMock.mockImplementation((_sessionId, _message, onEvent, signal) => {
      streamSignal = signal;
      onEvent({ event: "message", data: { reply: "Committed before Stop", seq: 2 } });
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        );
      });
    });
    const result = render(<ChatPanel sessionId={SESSION_ID} />);
    const textbox = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(textbox, { target: { value: "keep talking" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("Committed before Stop")).toBeInTheDocument();

    app.state = { ...app.state, stop_sequence: 2 };
    result.rerender(<ChatPanel sessionId={SESSION_ID} />);

    await waitFor(() => expect(streamSignal?.aborted).toBe(true));
    await waitFor(() => expect(getChatMessages).toHaveBeenCalledTimes(2));
    expect(screen.getByText("Committed before Stop").closest(".chat-message")).not.toHaveAttribute("data-streaming");
  });
});
