import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ChatRequestFailure } from "../api/chat-diagnostic-types";
import { t, translateKnown } from "../i18n";
import { useToast } from "../state/app-state";

// Records live in the backend; closing this disclosure only hides the view.
// There is no polling, model call or full-content fetch while collapsed.
export function ChatFailures({ sessionID, busy }: { sessionID: string; busy: boolean }) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<ChatRequestFailure[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setItems([]);
    setError("");
    if (!open || busy) return;
    const controller = new AbortController();
    setLoading(true);
    void api.chatFailures(sessionID, controller.signal).then(payload => {
      if (!controller.signal.aborted) setItems(payload.failures);
    }).catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [open, busy, sessionID]);
  return <details className="group chat-failures" open={open} onToggle={event => setOpen(event.currentTarget.open)}>
    <summary className="group-title">{t("Recent request failures")}</summary>
    {open && <div>
      <p className="hint-block">{t("The latest 24 failures are kept across chats. These records are never sent as conversation history.")}</p>
      {error && <p className="form-status" role="alert">{error}</p>}
      {loading && <p role="status">{t("Loading…")}</p>}
      {!busy && !loading && !error && items.length === 0 && <p className="hint-block">{t("No retained failures in this chat.")}</p>}
      {items.map(item => <FailureRecord key={`${sessionID}:${item.id}`} item={item} sessionID={sessionID} />)}
    </div>}
  </details>;
}

function FailureRecord({ item, sessionID }: { item: ChatRequestFailure; sessionID: string }) {
  const [includeContent, setIncludeContent] = useState(false);
  const [content, setContent] = useState<ChatRequestFailure["content"]>();
  const [busy, setBusy] = useState(false);
  const { show } = useToast();
  useEffect(() => {
    setContent(undefined);
    if (!includeContent) return;
    const controller = new AbortController();
    setBusy(true);
    void api.chatFailure(sessionID, item.id, true, controller.signal).then(record => {
      if (!controller.signal.aborted) setContent(record.content);
    }).catch(reason => {
      if (!controller.signal.aborted) show(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"), "error");
    }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    return () => controller.abort();
  }, [includeContent, item.id, sessionID, show]);
  async function copy() {
    try {
      const report = includeContent ? { ...item, content } : item;
      await navigator.clipboard.writeText(JSON.stringify(report, null, 2));
      show(t("Copied"));
    } catch { show(t("Clipboard unavailable."), "error"); }
  }
  return <details className="prompt-section">
    <summary><span>{translateKnown(item.error)}</span><time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time></summary>
    <dl className="prompt-inspector-summary">
      <div><dt>{t("Model")}</dt><dd>{item.diagnostics.provider} · {item.diagnostics.model}</dd></div>
      <div><dt>{t("LLM motion")}</dt><dd>{translateKnown(item.motion_mode)}</dd></div>
      <div><dt>{t("Reasoning")}</dt><dd>{translateKnown(item.reasoning_mode)}</dd></div>
      <div><dt>{t("Maximum output")}</dt><dd>{t("{count} tokens", { count: item.max_tokens })}</dd></div>
      <div><dt>{t("Prompt set")}</dt><dd>{item.diagnostics.prompt_set}</dd></div>
    </dl>
    {item.kind === "output_limit" && <p className="hint-block">{t("The model exhausted its output budget before finishing. Try reasoning Off for faster structured replies, or increase Maximum output in Model settings.")}</p>}
    <label className="field"><span><input type="checkbox" checked={includeContent} onChange={event => setIncludeContent(event.target.checked)} /> {t("Include prompt and model output")}</span></label>
    {includeContent && <p className="hint-block">{t("May contain private conversation details. Review before sharing. Credentials are redacted.")}</p>}
    <button type="button" className="btn btn-secondary" disabled={includeContent && (busy || !content)} onClick={() => void copy()}>{t("Copy diagnostics")}</button>
    {includeContent && content && <>
      {content.truncated && <p className="hint-block">{t("Content was shortened to the diagnostic limit.")}</p>}
      <details><summary>{t("Model output")}</summary><pre className="diagnostics-report">{content.raw_response}</pre>{content.repair_response && <pre className="diagnostics-report">{content.repair_response}</pre>}</details>
      <details><summary>{t("System prompt")}</summary><pre className="diagnostics-report">{content.system_prompt}</pre></details>
      <details><summary>{t("Response schema")}</summary><pre className="diagnostics-report">{content.response_schema}</pre></details>
    </>}
  </details>;
}
