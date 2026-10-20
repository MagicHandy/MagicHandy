import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { StartupReports } from "../api/startup-types";
import { t } from "../i18n";

export function StartupReportsPanel() {
  const [reports, setReports] = useState<StartupReports | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  useEffect(() => {
    const abort = new AbortController();
    void api.startupReports(abort.signal).then(setReports).catch(() => {
      if (!abort.signal.aborted) setUnavailable(true);
    });
    return () => abort.abort();
  }, []);

  function download() {
    const url = URL.createObjectURL(new Blob([JSON.stringify(reports, null, 2) + "\n"], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "magichandy-startup-report.json";
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  }

  return <div className="group">
    <h3 className="group-title">{t("Startup recovery")}</h3>
    <p className="hint-block">{t("Startup reports retain the latest failure and automatic port recovery. They exclude credentials, addresses, paths and chat. Review before sharing.")}</p>
    {unavailable && <p role="status">{t("Startup reports are unavailable. Check the startup folder in the app data directory.")}</p>}
    {reports && <>
      <p>{reports.current?.outcome === "recovered" ? t("MagicHandy selected an available local port automatically.") : reports.current?.outcome === "ready" ? t("The latest startup completed without port recovery.") : t("No completed startup report is available.")}</p>
      {reports.last_failure && <p>{t("A previous startup failure was retained.")} <time dateTime={reports.last_failure.finished_at}>{new Date(reports.last_failure.finished_at).toLocaleString()}</time></p>}
      {reports.last_recovery && <p>{t("An automatic port recovery was retained.")} <time dateTime={reports.last_recovery.finished_at}>{new Date(reports.last_recovery.finished_at).toLocaleString()}</time></p>}
      <button type="button" className="btn btn-secondary" onClick={download}>{t("Download startup report")}</button>
    </>}
  </div>;
}
