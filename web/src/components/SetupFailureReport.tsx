import { useState } from "react";
import { api } from "../api/client";
import type { SetupJob } from "../api/types";
import { t } from "../i18n";

export function SetupFailureReport({ job }: { job?: SetupJob }) {
  return job?.status === "failed" ? <FailureDownload key={job.id} id={job.id} /> : null;
}

function FailureDownload({ id }: { id: string }) {
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);

  async function download() {
    setPending(true);
    setFailed(false);
    try {
      const { blob, filename } = await api.setupFailureReport(id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      try { link.click(); }
      finally {
        link.remove();
        window.setTimeout(() => URL.revokeObjectURL(url), 0);
      }
    } catch {
      setFailed(true);
    } finally {
      setPending(false);
    }
  }

  return <div className="setup-failure-report">
    <p className="form-status">{t("Please share this report with the developer to help fix the installation failure.")}</p>
    <div><button type="button" className="btn btn-secondary" disabled={pending} onClick={() => void download()}>{pending ? t("Preparing report...") : t("Download failure report")}</button></div>
    {failed && <p className="form-status" role="alert">{t("Could not download the failure report. Please try again.")}</p>}
  </div>;
}
