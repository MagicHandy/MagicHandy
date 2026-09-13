import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { ChatLogMessage, ChatMessagesResponse } from "../api/types";
import { useChatHistory } from "./useChatHistory";

vi.mock("../api/client", () => ({ api: { getChatMessages: vi.fn(), advanceChatCursor: vi.fn() } }));
const getMessages = vi.mocked(api.getChatMessages);
const advanceCursor = vi.mocked(api.advanceChatCursor);
const sessionId = "recovery-conversation";
const row = (seq: number, revision = seq, content = `message ${seq}`, speech?: string): ChatLogMessage => ({
  seq, revision, content, role: "assistant", created_at: "now", speech_request_id: speech,
});
const page = (messages: ChatLogMessage[], revision: number, overrides: Partial<ChatMessagesResponse> = {}): ChatMessagesResponse => ({
  session_id: sessionId, messages, revision, next_revision: revision, cursor: 0, cursor_revision: 0,
  latest_seq: Math.max(0, ...messages.map((item) => item.seq)), first_seq: messages.length ? Math.min(...messages.map((item) => item.seq)) : 0,
  server_epoch: "process-one", has_more: false, reset: false, history_limit: 200, history_gap: false, ...overrides,
});
const options = () => ({ sessionId, epoch: "process-one" as string | undefined, backendOnline: true, readOnly: false, busy: false,
  busyRef: { current: false }, latestSeq: 0, revision: 0 as number | undefined, pollEpoch: 1, queueSpeech: vi.fn() });
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }

beforeEach(() => { getMessages.mockReset(); advanceCursor.mockReset(); advanceCursor.mockResolvedValue({ cursor: 0, session_id: sessionId }); });
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

describe("committed conversation recovery", () => {
  it("bounds catch-up work per state poll even when every page advertises more", async () => {
    getMessages.mockResolvedValueOnce(page([row(1)], 1, { reset: true }));
    const props = { ...options(), revision: 1, latestSeq: 1 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(1));
    getMessages.mockImplementation(async (_session, _after, request) => {
      const next = (request?.revision ?? 0) + 1;
      return page([row(next)], 200, { first_seq: 1, latest_seq: 200, next_revision: next, has_more: true });
    });
    view.rerender({ ...props, revision: 200, latestSeq: 200, pollEpoch: 2 });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(5));
    expect(getMessages).toHaveBeenCalledTimes(5);
    view.rerender({ ...props, revision: 200, latestSeq: 200, pollEpoch: 3 });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(9));
    expect(getMessages).toHaveBeenCalledTimes(9);
  });

  it("bounds a stalled read and releases timers after recovery and unmount", async () => {
    vi.useFakeTimers();
    getMessages.mockImplementationOnce((_session, _after, request) => new Promise((_resolve, reject) => {
      request?.signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
    })).mockResolvedValueOnce(page([row(1)], 1, { reset: true }));
    const props = options();
    const view = renderHook(useChatHistory, { initialProps: props });
    expect(getMessages).toHaveBeenCalledOnce();
    await act(async () => { await vi.advanceTimersByTimeAsync(15_001); });
    expect(view.result.current.historyError).toMatch(/timed out/);
    expect(getMessages.mock.calls[0][2]?.signal?.aborted).toBe(true);
    await act(async () => { view.rerender({ ...props, pollEpoch: 2 }); });
    expect(view.result.current.historyError).toBe("");
    expect(view.result.current.messages).toHaveLength(1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("backs off repeated failures instead of retrying on every state poll", async () => {
    vi.useFakeTimers();
    getMessages.mockRejectedValueOnce(new Error("offline")).mockRejectedValueOnce(new Error("still offline"))
      .mockResolvedValueOnce(page([row(1)], 1, { reset: true }));
    const props = options();
    const view = renderHook(useChatHistory, { initialProps: props });
    await act(async () => { await Promise.resolve(); });
    await act(async () => { view.rerender({ ...props, pollEpoch: 2 }); });
    expect(getMessages).toHaveBeenCalledTimes(2);
    await act(async () => { view.rerender({ ...props, pollEpoch: 3 }); });
    expect(getMessages).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); view.rerender({ ...props, pollEpoch: 4 }); });
    expect(view.result.current.messages).toHaveLength(1);
    expect(getMessages).toHaveBeenCalledTimes(3);
  });

  it("recovers a reply committed below the latest display sequence", async () => {
    getMessages.mockResolvedValueOnce(page([row(1, 1), row(3, 2)], 2, { reset: true }))
      .mockResolvedValueOnce(page([row(2, 3, "late committed reply", "tts-late")], 3, { latest_seq: 3, first_seq: 1 }));
    const props = options();
    const view = renderHook(useChatHistory, { initialProps: { ...props, latestSeq: 3, revision: 2 } });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(2));
    view.rerender({ ...props, latestSeq: 3, revision: 3, pollEpoch: 2 });
    await waitFor(() => expect(view.result.current.messages.map((item) => item.seq)).toEqual([1, 2, 3]));
    expect(getMessages).toHaveBeenLastCalledWith(sessionId, 3, expect.objectContaining({ revision: 2 }));
    expect(advanceCursor).toHaveBeenLastCalledWith(sessionId, 3, expect.objectContaining({ revision: 3, epoch: "process-one" }));
    expect(props.queueSpeech).toHaveBeenCalledOnce();
    expect(props.queueSpeech).toHaveBeenCalledWith("tts-late");
  });

  it("does not advance a legacy read position to an undelivered head", async () => {
    getMessages.mockResolvedValueOnce({ session_id: sessionId, messages: [row(1)], latest_seq: 2, cursor: 0 })
      .mockResolvedValueOnce({ session_id: sessionId, messages: [row(2)], latest_seq: 2, cursor: 1 });
    const props = { ...options(), epoch: undefined, revision: undefined, latestSeq: 2 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(advanceCursor).toHaveBeenCalledWith(sessionId, 1, expect.anything()));
    view.rerender({ ...props, pollEpoch: 2 });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(2));
    expect(getMessages).toHaveBeenLastCalledWith(sessionId, 1, expect.objectContaining({ revision: undefined }));
  });

  it("uses delivered revisions through bounded partial pages", async () => {
    getMessages.mockResolvedValueOnce(page([row(1)], 1, { reset: true }));
    for (let next = 2; next <= 4; next++) getMessages.mockResolvedValueOnce(page([row(next)], 4, { latest_seq: 4, first_seq: 1, next_revision: next, has_more: next < 4 }));
    const props = { ...options(), revision: 1, latestSeq: 1 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(1));
    view.rerender({ ...props, revision: 4, latestSeq: 4, pollEpoch: 2 });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(4));
    expect(getMessages.mock.calls.map((call) => call[2]?.revision)).toEqual([0, 1, 2, 3]);
    expect(advanceCursor).toHaveBeenLastCalledWith(sessionId, 4, expect.objectContaining({ revision: 4 }));
  });

  it("discards old process responses after a reconnect resync", async () => {
    const old = deferred<ChatMessagesResponse>();
    getMessages.mockReturnValueOnce(old.promise).mockResolvedValueOnce(page([row(5, 1, "new process")], 1, { reset: true, server_epoch: "process-two" }));
    const props = options();
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(getMessages).toHaveBeenCalledOnce());
    const oldSignal = getMessages.mock.calls[0][2]?.signal;
    view.rerender({ ...props, epoch: "process-two" });
    await waitFor(() => expect(view.result.current.messages[0]?.text).toBe("new process"));
    expect(oldSignal?.aborted).toBe(true);
    await act(async () => old.resolve(page([row(1, 1, "obsolete process")], 1, { reset: true })));
    expect(view.result.current.messages.map((item) => item.text)).toEqual(["new process"]);
    expect(advanceCursor.mock.calls.every((call) => call[2]?.epoch === "process-two")).toBe(true);
  });

  it("closes hidden reads and does not replay recovery speech on return", async () => {
    let visibility: DocumentVisibilityState = "visible";
    vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
    const tail = deferred<ChatMessagesResponse>();
    getMessages.mockResolvedValueOnce(page([row(1, 1, "earlier", "tts-old")], 1, { reset: true }))
      .mockReturnValueOnce(tail.promise)
      .mockResolvedValueOnce(page([row(1), row(2, 2, "recovered", "tts-recovered")], 2, { reset: true }));
    const props = { ...options(), revision: 1, latestSeq: 1 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(1));
    view.rerender({ ...props, revision: 2, latestSeq: 2, pollEpoch: 2 });
    await waitFor(() => expect(getMessages).toHaveBeenCalledTimes(2));
    const signal = getMessages.mock.calls[1][2]?.signal;
    act(() => { visibility = "hidden"; document.dispatchEvent(new Event("visibilitychange")); });
    expect(signal?.aborted).toBe(true);
    await act(async () => tail.resolve(page([row(2, 2, "obsolete tail", "tts-obsolete")], 2, { first_seq: 1 })));
    expect(view.result.current.messages).toHaveLength(1);
    act(() => { visibility = "visible"; document.dispatchEvent(new Event("visibilitychange")); });
    await waitFor(() => expect(view.result.current.messages.map((item) => item.seq)).toEqual([1, 2]));
    expect(props.queueSpeech).not.toHaveBeenCalled();
    expect(getMessages.mock.calls[2][2]?.revision).toBe(0);
  });

  it("rechecks observer status before queueing speech from a pending read", async () => {
    const tail = deferred<ChatMessagesResponse>();
    getMessages.mockResolvedValueOnce(page([row(1)], 1, { reset: true })).mockReturnValueOnce(tail.promise);
    const props = { ...options(), revision: 1, latestSeq: 1 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(1));
    view.rerender({ ...props, revision: 2, latestSeq: 2, pollEpoch: 2 });
    await waitFor(() => expect(getMessages).toHaveBeenCalledTimes(2));
    view.rerender({ ...props, revision: 2, latestSeq: 2, pollEpoch: 2, readOnly: true });
    await act(async () => tail.resolve(page([row(2, 2, "observer reply", "tts-observer")], 2, { first_seq: 1 })));
    expect(props.queueSpeech).not.toHaveBeenCalled();
    view.rerender({ ...props, revision: 2, latestSeq: 2, pollEpoch: 3 });
    act(() => view.result.current.queueSpeechOnce("tts-observer"));
    expect(props.queueSpeech).not.toHaveBeenCalled();
  });

  it("replaces stale rows and reports a retained-history gap", async () => {
    getMessages.mockResolvedValueOnce(page([row(1)], 1, { reset: true }))
      .mockResolvedValueOnce(page([row(40, 99, "retained one"), row(41, 100, "retained two", "tts-history")], 100, { reset: true, history_gap: true }));
    const props = { ...options(), revision: 1, latestSeq: 1 };
    const view = renderHook(useChatHistory, { initialProps: props });
    await waitFor(() => expect(view.result.current.messages).toHaveLength(1));
    view.rerender({ ...props, revision: 100, latestSeq: 41, pollEpoch: 2 });
    await waitFor(() => expect(view.result.current.messages.map((item) => item.seq)).toEqual([40, 41]));
    expect(view.result.current.historyNotice).toMatch(/older messages are no longer retained/);
    expect(props.queueSpeech).not.toHaveBeenCalled();
  });

  it("rejects malformed recovery metadata before acknowledging or playing", async () => {
    getMessages.mockResolvedValueOnce(page([row(1, 5, "invalid row", "tts-invalid")], 2, { reset: true, next_revision: 2 }));
    const view = renderHook(useChatHistory, { initialProps: options() });
    await waitFor(() => expect(view.result.current.historyError).toMatch(/Invalid conversation recovery/));
    expect(view.result.current.messages).toHaveLength(0);
    expect(advanceCursor).not.toHaveBeenCalled();
  });
});
