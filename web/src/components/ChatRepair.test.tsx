import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api, streamChat, streamChatRepair } from "../api/client";
import type { AppState, ChatStreamEvent } from "../api/types";
import { ChatPanel } from "./ChatPanel";

const app = vi.hoisted(() => ({
  state: {} as AppState,
  backendOnline: true,
  readOnly: false,
  show: vi.fn(), refresh: vi.fn(), queueSpeech: vi.fn(),
}));
vi.mock("../api/client", () => ({
  api: { getChatMessages: vi.fn(), advanceChatCursor: vi.fn() },
  streamChat: vi.fn(), streamChatRepair: vi.fn(),
}));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ ...app }), useToast: () => ({ show: app.show }),
}));
vi.mock("../state/voice-playback", () => ({ useVoicePlayback: () => ({ queueSpeech: app.queueSpeech }) }));

const sessionId = "repair-chat";
const failureText = "Phrase loop must keep real reversals.";
const rejectionLabel = "The model returned an invalid response. That response was not applied.";
const history = { session_id: sessionId, messages: [], latest_seq: 0, cursor: 0 };
const getMessages = vi.mocked(api.getChatMessages);
const send = vi.mocked(streamChat);
const repair = vi.mocked(streamChatRepair);

async function requestFailure() {
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled());
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "Change the pattern" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  return screen.findByRole("button", { name: "Repair response" });
}

describe("chat response repair", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    app.state = { stop_sequence: 7, uptime_seconds: 1, chat: { active_session_id: sessionId, latest_seq: 0 },
      settings: { llm: { conversation_connection_id: "gpt", hosted_response_repair_enabled: false }, voice: { enabled: false } } } as AppState;
    app.backendOnline = true; app.readOnly = false;
    app.refresh.mockResolvedValue(undefined);
    getMessages.mockReset(); getMessages.mockResolvedValue(history);
    vi.mocked(api.advanceChatCursor).mockResolvedValue({ cursor: 0, session_id: sessionId });
    send.mockReset(); repair.mockReset();
    send.mockImplementation(async (_session, _text, receive) => {
      receive({ event: "status", data: { state: "generating", user_seq: 1 } });
      receive({ event: "error", data: { message: failureText, repair: { failure_id: 17, available: true } } });
      receive({ event: "done", data: { ok: false, malformed: true } });
    });
  });

  it("uses the server failure handle once and replaces the failed response without duplicating the user turn", async () => {
    let finish!: () => void;
    let receiveRepair!: (event: ChatStreamEvent) => void;
    repair.mockImplementation((_session, _failure, receive) => {
      receiveRepair = receive;
      receive({ event: "status", data: { state: "repairing_response", provider: "chatgpt" } });
      return new Promise(resolve => { finish = resolve; });
    });
    render(<ChatPanel sessionId={sessionId} />);
    const action = await requestFailure();
    expect(screen.getByText(rejectionLabel)).toBeVisible();
    expect(screen.queryByText(failureText)).not.toBeInTheDocument();
    expect(app.show).not.toHaveBeenCalled();
    fireEvent.click(action);
    fireEvent.click(action);
    expect(repair).toHaveBeenCalledOnce();
    expect(repair).toHaveBeenCalledWith(sessionId, 17, expect.any(Function), expect.any(AbortSignal), 7, undefined);
    expect(screen.getByText("Repairing response…", { selector: ".form-status" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "Next request draft" } });
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), { key: "Enter" });
    expect(screen.getByRole("textbox", { name: "Message" })).toHaveValue("Next request draft");
    expect(send).toHaveBeenCalledOnce();
    act(() => receiveRepair({ event: "repair_delta", data: { text: '{"reply":"Unvalidated repair"' } }));
    expect(screen.queryByText("Unvalidated repair")).not.toBeInTheDocument();
    expect(screen.getByText(rejectionLabel)).toBeVisible();
    await act(async () => {
      receiveRepair({ event: "message", data: { reply: "Corrected pattern", seq: 2, initial_malformed: true, diagnostics: { repaired: true } } });
      receiveRepair({ event: "done", data: { ok: true } });
      finish();
    });
    expect(screen.getAllByText("Change the pattern")).toHaveLength(1);
    expect(screen.getAllByText("Corrected pattern")).toHaveLength(1);
    expect(screen.queryByText(failureText)).not.toBeInTheDocument();
    expect(screen.getByText("Corrected pattern").closest(".chat-message")).not.toHaveAttribute("data-state", "warning");
    expect(screen.queryByText("Response repaired.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show response diagnostics" })).toHaveAttribute("title", expect.stringContaining("Repaired response"));
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
  });

  it("shows auto-repair progress and never offers a second attempt after failed repair", async () => {
    let finish!: () => void;
    let receiveTurn!: (event: ChatStreamEvent) => void;
    send.mockImplementation((_session, _text, receive) => {
      receiveTurn = receive;
      receive({ event: "delta", data: { text: '{"reply":"Rejected draft"}' } });
      receive({ event: "status", data: { state: "repairing_response" } });
      return new Promise(resolve => { finish = resolve; });
    });
    render(<ChatPanel sessionId={sessionId} />);
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled());
    fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "New motion" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(screen.getByText("Repairing response…", { selector: ".form-status" })).toBeVisible();
    expect(screen.queryByText("Rejected draft")).not.toBeInTheDocument();
    await act(async () => {
      receiveTurn({ event: "error", data: { message: "Repair still invalid" } });
      receiveTurn({ event: "done", data: { ok: false, malformed: true } });
      finish();
    });
    expect(screen.getByText("Repair still invalid")).toBeVisible();
    expect(screen.getByText("Repair did not succeed. You can send a new request.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(repair).not.toHaveBeenCalled();
  });

  it("does not invent repair eligibility for a refusal or network error", async () => {
    send.mockImplementation(async (_session, _text, receive) => {
      receive({ event: "error", data: { message: "Provider declined request" } });
      receive({ event: "done", data: { ok: false } });
    });
    render(<ChatPanel sessionId={sessionId} />);
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled());
    fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "Hello" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("Provider declined request")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
  });

  it("keeps the live first-turn failure when same-process history recovery returns a reset", async () => {
    app.state.observation = { epoch: "repair-process", revision: 1, observed_at: "now" };
    const metadata = { session_id: sessionId, server_epoch: "repair-process", has_more: false, cursor: 0, cursor_revision: 0, reset: true };
    getMessages.mockResolvedValueOnce({ ...metadata, first_seq: 0, latest_seq: 0, revision: 0, next_revision: 0, messages: [] })
      .mockResolvedValue({ ...metadata, first_seq: 1, latest_seq: 1, revision: 1, next_revision: 1,
        messages: [{ seq: 1, revision: 1, role: "user", content: "Change the pattern", created_at: "now" }] });
    render(<ChatPanel sessionId={sessionId} />);
    await requestFailure();
    await waitFor(() => expect(getMessages).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: "Repair response" })).toBeVisible();
    expect(screen.getByText(rejectionLabel)).toBeVisible();
    expect(screen.getAllByText("Change the pattern")).toHaveLength(1);
  });

  it.each(["Stop", "ownership", "another turn"])("invalidates a pending repair after %s changes", async (change) => {
    const view = render(<ChatPanel sessionId={sessionId} />);
    await requestFailure();
    if (change === "Stop") app.state = { ...app.state, stop_sequence: 8 };
    if (change === "ownership") app.state = { ...app.state, controller: { active: false, read_only: true, generation: 2 } };
    if (change === "another turn") app.state = { ...app.state, chat: { active_session_id: sessionId, latest_seq: 2 } };
    view.rerender(<ChatPanel sessionId={sessionId} />);
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(screen.getByText("Repair is no longer available. Send a new request.")).toBeVisible();
    expect(repair).not.toHaveBeenCalled();
  });

  it("locks the action for read-only clients and drops it on a new turn", async () => {
    const view = render(<ChatPanel sessionId={sessionId} />);
    await requestFailure();
    app.readOnly = true;
    view.rerender(<ChatPanel sessionId={sessionId} />);
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(repair).not.toHaveBeenCalled();
    app.readOnly = false;
    view.rerender(<ChatPanel sessionId={sessionId} />);
    send.mockImplementation(async (_session, _text, receive) => { receive({ event: "message", data: { reply: "A new answer" } }); receive({ event: "done", data: { ok: true } }); });
    fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "Different request" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("A new answer")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
  });

  it("consumes a stale server rejection and keeps the reason visible", async () => {
    repair.mockRejectedValue(Object.assign(new Error("Motion context changed. Send a new request."), { status: 409 }));
    render(<ChatPanel sessionId={sessionId} />);
    fireEvent.click(await requestFailure());
    expect(await screen.findByText("Motion context changed. Send a new request.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(screen.getByText("Repair is no longer available. Send a new request.")).toBeVisible();
    expect(screen.getByRole("textbox", { name: "Message" })).toBeEnabled();
  });

  it("aborts a running repair on Stop and retains the original failure without a retry action", async () => {
    let signal: AbortSignal | undefined;
    repair.mockImplementation((_session, _failure, receive, abort) => {
      signal = abort;
      receive({ event: "status", data: { state: "repairing_response" } });
      return new Promise((_resolve, reject) => abort?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true }));
    });
    const view = render(<ChatPanel sessionId={sessionId} />);
    fireEvent.click(await requestFailure());
    app.state = { ...app.state, stop_sequence: 8 };
    view.rerender(<ChatPanel sessionId={sessionId} />);
    await waitFor(() => expect(signal?.aborted).toBe(true));
    expect(screen.getByText(rejectionLabel)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(screen.getByText("Repair is no longer available. Send a new request.")).toBeVisible();
  });

  it("leaves settings eligibility to the backend and echoes the current video motion owner", async () => {
    repair.mockResolvedValue(undefined);
    const view = render(<ChatPanel sessionId={sessionId} motionOwner="off" />);
    await requestFailure();
    app.state = { ...app.state, settings: { ...app.state.settings!, voice: { ...app.state.settings!.voice, enabled: true } } };
    view.rerender(<ChatPanel sessionId={sessionId} motionOwner="off" />);
    fireEvent.click(screen.getByRole("button", { name: "Repair response" }));
    expect(repair).toHaveBeenCalledWith(sessionId, 17, expect.any(Function), expect.any(AbortSignal), 7, "off");
  });

  it("drops a live offer if the video changes its motion source", async () => {
    const view = render(<ChatPanel sessionId={sessionId} />);
    await requestFailure();
    view.rerender(<ChatPanel sessionId={sessionId} motionOwner="script" />);
    expect(screen.queryByRole("button", { name: "Repair response" })).not.toBeInTheDocument();
    expect(repair).not.toHaveBeenCalled();
  });

  it("keeps a transient error beside its user turn after a later reply is committed", async () => {
    getMessages.mockResolvedValueOnce(history).mockResolvedValue({ ...history, latest_seq: 1,
      messages: [{ seq: 1, role: "user", content: "Change the pattern", created_at: "now" }] });
    render(<ChatPanel sessionId={sessionId} />);
    await requestFailure();
    getMessages.mockResolvedValue({ ...history, latest_seq: 3, messages: [
      { seq: 2, role: "user", content: "New request", created_at: "now" },
      { seq: 3, role: "assistant", content: "New answer", created_at: "now" },
    ] });
    send.mockImplementation(async (_session, _text, receive) => {
      receive({ event: "status", data: { state: "generating", user_seq: 2 } });
      receive({ event: "message", data: { reply: "New answer", seq: 3 } });
      receive({ event: "done", data: { ok: true } });
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "New request" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("New answer");
    await waitFor(() => expect(getMessages).toHaveBeenCalledTimes(3));
    const messages = [...document.querySelectorAll(".chat-bubble")].map(element => element.textContent);
    expect(messages).toEqual(["Change the pattern", rejectionLabel, "New request", "New answer"]);
  });
});
