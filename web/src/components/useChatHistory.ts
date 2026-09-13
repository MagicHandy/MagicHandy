import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ChatMessageDiagnostics, ChatMessagesResponse } from "../api/types";
import { t, translateKnown } from "../i18n";

export interface ChatDisplayMessage {
  id: string;
  seq?: number;
  role: "user" | "assistant";
  text: string;
  streaming?: boolean;
  warning?: boolean;
  diagnostics?: ChatMessageDiagnostics;
}

interface HistoryOptions {
  sessionId: string;
  epoch?: string;
  backendOnline: boolean;
  readOnly: boolean;
  busy: boolean;
  busyRef: { current: boolean };
  latestSeq: number;
  revision?: number;
  pollEpoch: number;
  queueSpeech: (id: string) => void;
}

const HISTORY_LIMIT = 200;
const READ_TIMEOUT_MS = 15_000;
const MAX_PAGES_PER_READ = 4;
const pageVisible = () => document.visibilityState !== "hidden";
const nonnegative = (value: unknown): value is number => Number.isSafeInteger(value) && Number(value) >= 0;
const failure = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Conversation history request failed.");

// The browser holds a rendered cache of committed rows, plus local stream
// placeholders. Recovery positions come from delivered backend revisions;
// streaming display sequences never advance this read position.
export function useChatHistory(options: HistoryOptions) {
  const latest = useRef(options);
  useLayoutEffect(() => { latest.current = options; });
  const [messages, setMessages] = useState<ChatDisplayMessage[]>([]);
  const [historyLoading, setHistoryLoading] = useState(true);
  const [historyError, setHistoryError] = useState("");
  const [tailError, setTailError] = useState("");
  const [historyNotice, setHistoryNotice] = useState("");
  const alive = useRef(false);
  const generation = useRef(0);
  const seeded = useRef(false);
  const needsTail = useRef(false);
  const position = useRef<{ seq: number; revision?: number; epoch?: string }>({ seq: 0 });
  const inFlight = useRef<{ controller: AbortController; promise: Promise<void>; full: boolean } | null>(null);
  const acknowledgement = useRef<{ controller: AbortController; timer: number } | null>(null);
  const speechSeen = useRef(new Set<string>());
  const failureCount = useRef(0);
  const retryAt = useRef(0);

  const cancelReads = useCallback(() => {
    generation.current += 1;
    inFlight.current?.controller.abort();
    inFlight.current = null;
    acknowledgement.current?.controller.abort();
    window.clearTimeout(acknowledgement.current?.timer);
    acknowledgement.current = null;
  }, []);

  const queueSpeechOnce = useCallback((id: string) => {
    if (!id || speechSeen.current.has(id)) return;
    speechSeen.current.add(id);
    if (speechSeen.current.size > 256) speechSeen.current.delete(speechSeen.current.values().next().value!);
    if (latest.current.readOnly || !latest.current.backendOnline || !pageVisible()) return;
    latest.current.queueSpeech(id);
  }, []);

  const acknowledge = useCallback(() => {
    const read = position.current;
    if (read.seq === 0 && !read.revision) return;
    acknowledgement.current?.controller.abort();
    window.clearTimeout(acknowledgement.current?.timer);
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
    const entry = { controller, timer };
    acknowledgement.current = entry;
    void api.advanceChatCursor(latest.current.sessionId, read.seq, { revision: read.revision, epoch: read.epoch, signal: controller.signal })
      .catch(() => undefined).finally(() => {
        window.clearTimeout(timer);
        if (acknowledgement.current === entry) acknowledgement.current = null;
      });
  }, []);

  const read = useCallback((full: boolean, suppressSpeech = full): Promise<void> => {
    if (!alive.current || !latest.current.backendOnline || !pageVisible() || (!full && (latest.current.busyRef.current || !seeded.current))) return Promise.resolve();
    if (inFlight.current && (!full || inFlight.current.full)) return inFlight.current.promise;
    if (full) cancelReads();
    const token = generation.current;
    const requestedSession = latest.current.sessionId;
    const controller = new AbortController();
    const current = () => alive.current && generation.current === token && latest.current.sessionId === requestedSession;
    if (full) { setHistoryLoading(true); setHistoryError(""); }
    const timer = window.setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
    const promise = (async () => {
      let reset = full;
      let recovering = suppressSpeech;
      let needsAcknowledgement = false;
      try {
        for (let batch = 0; batch < MAX_PAGES_PER_READ; batch++) {
          const before = position.current;
          const after = reset ? 0 : before.seq;
          const afterRevision = reset ? 0 : before.revision;
          const page = await api.getChatMessages(requestedSession, after, { revision: afterRevision, signal: controller.signal });
          if (!current()) return;
          if (controller.signal.aborted) throw new Error("Conversation history request timed out.");
          if (!full && latest.current.busyRef.current) return;
          const modern = validatePage(page, requestedSession, latest.current.epoch);
          if (!reset && modern && before.epoch && before.epoch !== page.server_epoch) {
            // A tail from another process cannot establish a complete history.
            position.current = { seq: 0 };
            reset = true;
            recovering = true;
            continue;
          }
          const replace = reset || page.reset === true;
          const deliveredSeq = page.messages.reduce((seq, item) => Math.max(seq, item.seq), replace ? 0 : before.seq);
          const next = modern ? page.next_revision! : undefined;
          if (modern && !replace && (next! < (before.revision ?? 0) || (page.has_more && next! <= (before.revision ?? 0)))) {
            throw new Error("Invalid conversation recovery response.");
          }
          setMessages((existing) => mergeMessages(existing, page, replace));
          position.current = { seq: deliveredSeq, revision: next, epoch: modern ? page.server_epoch : undefined };
          needsAcknowledgement = deliveredSeq > page.cursor || (modern && next! > (page.cursor_revision ?? 0));
          seeded.current = true;
          failureCount.current = 0;
          retryAt.current = 0;
          needsTail.current = modern ? page.has_more === true : page.latest_seq > deliveredSeq;
          setHistoryError("");
          setTailError(!modern && !page.messages.length && needsTail.current ? t("Conversation updates could not be synchronized; retrying.") : "");
          if (page.history_gap) setHistoryNotice(t("Some older messages are no longer retained. Showing the available conversation history."));
          else if (replace) setHistoryNotice(page.messages.length >= HISTORY_LIMIT ? t("Showing the latest {count} retained messages.", { count: HISTORY_LIMIT }) : "");
          // Recovery/resets never replay old speech. Normal tail delivery and
          // live SSE share the same bounded request-ID deduplication.
          for (const message of page.messages) if (message.speech_request_id) {
            if (!recovering && !replace) queueSpeechOnce(message.speech_request_id);
            else speechSeen.current.add(message.speech_request_id);
          }
          while (speechSeen.current.size > 256) speechSeen.current.delete(speechSeen.current.values().next().value!);
          if (!modern || !page.has_more) break;
          reset = false;
        }
        if (current() && needsAcknowledgement) acknowledge();
      } catch (error) {
        if (!current()) return;
        const reason = controller.signal.aborted ? new Error("Conversation history request timed out.") : error;
        needsTail.current = true;
        failureCount.current += 1;
        retryAt.current = Date.now() + (failureCount.current === 1 ? 0 : Math.min(30_000, 1000 * 2 ** Math.min(failureCount.current, 5)));
        if (full) { seeded.current = false; setHistoryError(failure(reason)); }
        else setTailError(t("Conversation updates delayed: {reason} Retrying.", { reason: failure(reason) }));
      } finally {
        window.clearTimeout(timer);
        if (inFlight.current?.controller === controller) inFlight.current = null;
        if (current() && full) setHistoryLoading(false);
      }
    })();
    inFlight.current = { controller, promise, full };
    return promise;
  }, [acknowledge, cancelReads, queueSpeechOnce]);

  const loadHistory = useCallback(() => read(true), [read]);
  const loadTail = useCallback(() => read(false), [read]);
  const reconcileHistory = useCallback(() => read(false, true), [read]);

  useEffect(() => {
    alive.current = true;
    seeded.current = false;
    position.current = { seq: 0 };
    needsTail.current = false;
    failureCount.current = 0;
    retryAt.current = 0;
    speechSeen.current.clear();
    cancelReads();
    void loadHistory();
    return () => { alive.current = false; cancelReads(); };
  }, [options.sessionId, options.epoch, options.backendOnline, loadHistory, cancelReads]);

  useEffect(() => {
    const changed = () => {
      cancelReads();
      if (pageVisible()) { seeded.current = false; void loadHistory(); }
    };
    document.addEventListener("visibilitychange", changed);
    return () => document.removeEventListener("visibilitychange", changed);
  }, [cancelReads, loadHistory]);

  useEffect(() => {
    if (options.busy || Date.now() < retryAt.current) return;
    if (!seeded.current) { if (needsTail.current) void loadHistory(); return; }
    const newer = position.current.revision !== undefined && options.revision !== undefined
      ? options.revision > position.current.revision : options.latestSeq > position.current.seq;
    if (needsTail.current || newer) void loadTail();
  }, [options.busy, options.latestSeq, options.revision, options.pollEpoch, loadHistory, loadTail]);

  return { messages, setMessages, historyLoading, historyError, tailError, historyNotice, loadHistory, loadTail, reconcileHistory, cancelReads, queueSpeechOnce };
}

function validatePage(page: ChatMessagesResponse, sessionId: string, expectedEpoch?: string): boolean {
  if (!page || page.session_id !== sessionId || !nonnegative(page.latest_seq) || !Array.isArray(page.messages) || page.messages.length > HISTORY_LIMIT ||
      page.messages.some((item) => !nonnegative(item.seq) || item.seq === 0 || item.seq > page.latest_seq || (item.role !== "user" && item.role !== "assistant") || typeof item.content !== "string")) {
    throw new Error("Invalid conversation recovery response.");
  }
  const modern = page.revision !== undefined || page.next_revision !== undefined || page.server_epoch !== undefined;
  if (modern && (!nonnegative(page.revision) || !nonnegative(page.next_revision) || page.next_revision > page.revision ||
      !page.server_epoch || typeof page.server_epoch !== "string" || !nonnegative(page.first_seq) || typeof page.has_more !== "boolean" || typeof page.reset !== "boolean")) {
    throw new Error("Invalid conversation recovery response.");
  }
  if (modern && ((page.latest_seq === 0 ? page.first_seq !== 0 : page.first_seq! <= 0 || page.first_seq! > page.latest_seq) ||
      page.messages.some((item) => !nonnegative(item.revision) || item.revision === 0 || item.revision > page.next_revision! || item.seq < page.first_seq!) ||
      new Set(page.messages.map((item) => item.seq)).size !== page.messages.length)) throw new Error("Invalid conversation recovery response.");
  if (expectedEpoch && (!modern || expectedEpoch !== page.server_epoch)) throw new Error("Conversation changed while reconnecting. Reload its history.");
  return modern;
}

function mergeMessages(existing: ChatDisplayMessage[], page: ChatMessagesResponse, replace: boolean): ChatDisplayMessage[] {
  const bySequence = new Map<number, ChatDisplayMessage>();
  const transient: ChatDisplayMessage[] = [];
  if (!replace) for (const item of existing) {
    if (item.seq !== undefined) { if (item.seq >= (page.first_seq ?? 0)) bySequence.set(item.seq, item); }
    else transient.push(item);
  }
  for (const message of page.messages) {
    const previous = bySequence.get(message.seq);
    bySequence.set(message.seq, {
      id: `log-${message.seq}`, seq: message.seq, role: message.role, text: message.content,
      diagnostics: message.diagnostics, warning: previous?.warning,
    });
  }
  return [...[...bySequence.values()].sort((a, b) => a.seq! - b.seq!), ...transient].slice(-HISTORY_LIMIT);
}
