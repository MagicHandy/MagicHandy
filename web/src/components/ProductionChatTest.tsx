import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ChatRequestFailure } from "../api/chat-diagnostic-types";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";

export function ProductionChatTest() {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{ valid: boolean; report: ChatRequestFailure }>();
  const active = useRef<AbortController>();
  useEffect(() => {
    const stop = () => active.current?.abort();
    window.addEventListener("magichandy:emergency-stop", stop);
    return () => { stop(); window.removeEventListener("magichandy:emergency-stop", stop); };
  }, []);
  async function run() {
    if (busy || !message.trim()) return;
    const controller = new AbortController();
    active.current = controller;
    setBusy(true); setError(""); setResult(undefined);
    try {
      const next = await api.productionChatTest(message, controller.signal);
      if (!controller.signal.aborted) setResult(next);
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    } finally { setBusy(false); active.current = undefined; }
  }
  async function copy() {
    try { await navigator.clipboard.writeText(JSON.stringify(result, null, 2)); show(t("Copied")); }
    catch { show(t("Clipboard unavailable."), "error"); }
  }
  return <details className="prompt-exact">
    <summary>{t("Test active chat configuration")}</summary>
    <p className="hint-block">{t("Uses the saved model, persona, history and reasoning. No motion is applied and no dialogue is saved.")}</p>
    <label className="field"><span className="label">{t("Test message")}</span><textarea value={message} rows={2} maxLength={1000} disabled={busy} onChange={event => setMessage(event.target.value)} /></label>
    <div className="row-actions"><button type="button" className="btn btn-secondary" disabled={!backendOnline || busy || !message.trim()} onClick={() => void run()}>{t("Test without motion")}</button>
      {busy && <button type="button" className="btn btn-secondary" onClick={() => active.current?.abort()}>{t("Cancel")}</button>}
    </div>
    {busy && <p role="status">{t("Generating")}</p>}
    {error && <p className="form-status" role="alert">{error}</p>}
    {result && <>
      <p role="status">{result.valid ? t("Response accepted. Motion was not applied.") : translateKnown(result.report.error)}</p>
      {result.report.kind === "output_limit" && <p className="hint-block">{t("The model exhausted its output budget before finishing. Try reasoning Off for faster structured replies, or increase Maximum output in Model settings.")}</p>}
      <pre className="diagnostics-report production-preview-output">{result.report.content?.raw_response}</pre>
      <button type="button" className="btn btn-secondary" onClick={() => void copy()}>{t("Copy test report")}</button>
      <p className="hint-block">{t("May contain private conversation details. Review before sharing. Credentials are redacted.")}</p>
    </>}
  </details>;
}
