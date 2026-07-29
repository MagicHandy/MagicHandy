import { t, translateKnown } from "../i18n";
// Diagnostics: one copyable plain-text report, a trace export, and a
// double-confirm settings reset. Each is a separate group because they are
// separate things — reading state, exporting a file, and destroying settings.
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { MediaToolStatus, PublicSettings } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { buildDiagnosticsReport } from "./diagnostics-report";

const msg = (e: unknown) => (e instanceof Error ? translateKnown(e.message) : t("Request failed"));

function download(name: string, content: string) {
  const blob = new Blob([content], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

function resetSettingsFromError(error: unknown): PublicSettings | null {
  if (!error || typeof error !== "object" || !("body" in error)) return null;
  const body = error.body;
  if (!body || typeof body !== "object" || !("settings" in body)) return null;
  return body.settings as PublicSettings;
}

export function DiagnosticsPanel({
  locked = false,
  onReset,
}: {
  locked?: boolean;
  onReset?: (settings: PublicSettings) => void | Promise<void>;
}) {
  const { state, backendOnline, refresh } = useAppState();
  const { show } = useToast();
  const [confirmReset, setConfirmReset] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [tools, setTools] = useState<MediaToolStatus | null>(null);
  // Regenerating on a timestamp rather than on every render keeps the report
  // stable while it is being read and selected.
  const [generatedAt, setGeneratedAt] = useState(() => new Date());

  const loadTools = useCallback(async () => {
    try {
      const response = await api.mediaTools();
      setTools(response.tools);
    } catch {
      // FFmpeg status is one line of the report; failing to read it must not
      // cost the user the other six sections.
      setTools(null);
    }
  }, []);

  useEffect(() => { void loadTools(); }, [loadTools]);

  const report = useMemo(
    () => buildDiagnosticsReport({ state, tools, generatedAt }),
    [state, tools, generatedAt],
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(report);
      show(t("Diagnostics report copied."));
    } catch {
      show(t("Clipboard unavailable."), "error");
    }
  }

  async function regenerate() {
    refresh();
    await loadTools();
    setGeneratedAt(new Date());
  }

  async function exportTrace() {
    try {
      const data = await api.exportTrace();
      download("magichandy-trace.json", JSON.stringify(data, null, 2));
    } catch (e) {
      show(msg(e), "error");
    }
  }

  async function reset() {
    if (resetting) return;
    if (!confirmReset) {
      setConfirmReset(true);
      return;
    }
    setConfirmReset(false);
    setResetting(true);
    try {
      const response = await api.resetSettings();
      await onReset?.(response.settings);
      show(t("Settings reset to defaults."));
      refresh();
    } catch (e) {
      const resetSettings = resetSettingsFromError(e);
      if (resetSettings) {
        await onReset?.(resetSettings);
        refresh();
      }
      show(msg(e), "error");
    } finally {
      setResetting(false);
    }
  }

  return (
    <>
      <div className="group">
        <h3 className="group-title">{t("Status report")}</h3>
        <p className="hint-block">{t("Everything below is what gets copied. No keys or credentials are included.")}</p>
        <div className="row-actions">
          <button type="button" className="btn btn-primary" onClick={() => void copy()}>{t("Copy report")}</button>
          <button type="button" className="btn btn-secondary" disabled={!backendOnline} onClick={() => void regenerate()}>{t("Refresh")}</button>
        </div>
        <pre className="diagnostics-report" aria-label={t("Diagnostics report")}>{report}</pre>
      </div>

      <div className="group">
        <h3 className="group-title">{t("Trace export")}</h3>
        <p className="hint-block">{t("A detailed machine-readable capture of recent runtime activity, for attaching to a bug report.")}</p>
        <button type="button" className="btn btn-secondary" disabled={!backendOnline} onClick={() => void exportTrace()}>{t("Export trace")}</button>
      </div>

      <div className="group">
        <h3 className="group-title">{t("Reset")}</h3>
        <p className="hint-block">{t("Restores every setting to factory defaults, including the connection key. Saved memories and prompt sets are not touched.")}</p>
        <button type="button" className="btn btn-danger-outline" disabled={locked || resetting} onClick={() => void reset()}>
          {resetting ? t("Resetting settings") : confirmReset ? t("Confirm reset all settings") : t("Reset all settings")}
        </button>
      </div>
    </>
  );
}
