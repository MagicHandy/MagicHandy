import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import { defaultDiagnosticReportOptions, type DiagnosticReportOptions } from "../api/diagnostic-report-types";
import { t, translateKnown } from "../i18n";

type Flag = Exclude<keyof DiagnosticReportOptions, "chat">;

export function DiagnosticReportDownload({ online }: { online: boolean }) {
  const [options, setOptions] = useState({ ...defaultDiagnosticReportOptions });
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  const menuID = useId();
  useEffect(() => () => pending.current?.abort(), []);

  const selected = [t("Status")];
  if (options.settings) selected.push(t("Settings"));
  if (options.traces) selected.push(t("Motion traces"));
  if (options.chat === "failures") selected.push(t("Only chat failures"));
  if (options.chat === "full") selected.push(t("Full chat history"), t("Chat failures"));
  if (options.model_output && options.chat !== "none") selected.push(t("Failed model output"));
  if (options.personas) selected.push(t("Personas and lore"));
  if (options.prompts) selected.push(t("Prompts"));
  if (options.memories) selected.push(t("Saved memories"));

  const toggle = (key: Flag, label: string, hint?: string, disabled = false) => (
    <label className="diagnostic-option" key={key}>
      <span className="diagnostic-option-copy"><span>{label}</span>{hint && <small>{hint}</small>}</span>
      <span className="toggle">
        <input type="checkbox" role="switch" aria-label={label} checked={options[key]} disabled={busy || disabled}
          onChange={event => setOptions(current => ({ ...current, [key]: event.target.checked }))} />
        <span className="track" aria-hidden="true" />
      </span>
    </label>
  );

  async function download() {
    if (pending.current || !online) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError("");
    try {
      const { blob, filename } = await api.exportDiagnosticReport(options, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      try { link.click(); }
      finally {
        link.remove();
        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
    } catch (failure) {
      if (!controller.signal.aborted) setError(failure instanceof Error ? translateKnown(failure.message) : t("Request failed"));
    } finally {
      pending.current = null;
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  return <div className="diagnostic-download" aria-busy={busy || undefined}>
    <div className="row-actions">
      <button type="button" className="btn btn-primary" disabled={!online || busy} onClick={() => void download()}>
        {busy ? t("Preparing report…") : t("Download diagnostic report")}
      </button>
      <button type="button" className="btn btn-secondary" aria-expanded={expanded} aria-controls={menuID}
        onClick={() => setExpanded(value => !value)}>{t("Customize report")}</button>
    </div>
    <p className="hint-block diagnostic-selection">{t("Includes: {items}", { items: selected.join(", ") })}</p>
    <p className="hint-block">{t("Downloads a ZIP to your device. Credentials and local paths are redacted. Nothing is sent automatically.")}</p>
    {expanded && <fieldset id={menuID} className="diagnostic-options" disabled={busy}>
      <legend className="visually-hidden">{t("Report contents")}</legend>
      {toggle("settings", t("Settings"), t("Model, device, motion, voice and app configuration."))}
      {toggle("traces", t("Motion traces"), t("Recent runtime activity and the last retained stopped run."))}
      <label className="diagnostic-option">
        <span className="diagnostic-option-copy"><span>{t("Chat content")}</span><small>{t("Full history includes all retained chats. Deleted or pruned messages are unavailable.")}</small></span>
        <select aria-label={t("Chat content")} value={options.chat} onChange={event => {
          const chat = event.target.value as DiagnosticReportOptions["chat"];
          setOptions(current => ({ ...current, chat, model_output: chat === "none" ? false : current.model_output }));
        }}>
          <option value="none">{t("No chat content")}</option>
          <option value="failures">{t("Only chat failures")}</option>
          <option value="full">{t("Full chat history")}</option>
        </select>
      </label>
      {toggle("model_output", t("Failed model output"), t("Adds raw and repair responses from failed requests."), options.chat === "none")}
      {toggle("personas", t("Personas and lore"), t("Saved personas, descriptions and lore. Portraits are excluded."))}
      {toggle("prompts", t("Prompts"), t("Prompt sets and captured system prompts. Captured prompts may also contain chat, persona and memory text."))}
      {toggle("memories", t("Saved memories"), t("Includes enabled and disabled memory entries."))}
    </fieldset>}
    {error && <p className="form-status" role="alert">{error}</p>}
  </div>;
}
