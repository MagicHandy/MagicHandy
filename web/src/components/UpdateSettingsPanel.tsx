import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { UpdateStatus } from "../api/types";
import { t } from "../i18n";

interface Props {
  currentVersion?: string;
  automatic: boolean;
  preferenceDisabled: boolean;
  checkDisabled: boolean;
  onAutomaticChange: (automatic: boolean) => void;
}

export function UpdateSettingsPanel({ currentVersion, automatic, preferenceDisabled, checkDisabled, onAutomaticChange }: Props) {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [checking, setChecking] = useState(false);
  const releaseBuild = /^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(currentVersion?.trim() ?? "");

  useEffect(() => {
    if (!automatic || !releaseBuild) return;
    let cancelled = false;
    void api.updateStatus().then((next) => {
      if (!cancelled) setStatus(next);
    }).catch(() => {
      // Automatic failures remain quiet; Check now reports them in place.
    });
    return () => {
      cancelled = true;
    };
  }, [automatic, releaseBuild]);

  async function checkNow() {
    if (checking) return;
    setChecking(true);
    try {
      setStatus(await api.updateStatus(true));
    } catch {
      setStatus({
        state: "error",
        current_version: currentVersion ?? "dev",
        message: t("GitHub Releases could not be checked. Try again later."),
      });
    } finally {
      setChecking(false);
    }
  }

  const installed = currentVersion?.trim() || status?.current_version || t("Development build");
  return (
    <div className="update-settings">
      <div className="update-version-row">
        <span>{t("Installed version")}</span>
        <strong>{installed}</strong>
      </div>
      <label className="toggle-line hint-block">
        <span className="toggle">
          <input
            type="checkbox"
            checked={automatic}
            disabled={preferenceDisabled}
            onChange={(event) => onAutomaticChange(event.target.checked)}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{t("Check for updates automatically")}</span>
      </label>
      <p className="hint-block">{t("Automatic checks contact GitHub Releases after the app starts. No account or token is required.")}</p>
      <div className="actions update-actions">
        <button type="button" className="btn btn-secondary" disabled={checkDisabled || checking} onClick={() => void checkNow()}>
          {checking ? t("Checking for updates...") : t("Check now")}
        </button>
        {status?.state === "available" && status.latest && (
          <a className="btn btn-primary" href={status.latest.url} target="_blank" rel="noreferrer">
            {t("View release")}
          </a>
        )}
      </div>
      {status && <UpdateResult status={status} />}
    </div>
  );
}

function UpdateResult({ status }: { status: UpdateStatus }) {
  const latestVersion = status.latest?.version ?? status.latest?.tag ?? "";
  let title = t("Update check failed");
  let detail = t("GitHub Releases could not be checked. Try again later.");
  let tone = "error";
  if (status.state === "available") {
    title = t("Update available");
    detail = t("MagicHandy {version} is available.", { version: latestVersion });
    tone = "info";
  } else if (status.state === "current") {
    title = t("MagicHandy is up to date");
    detail = t("{version} is the latest stable release.", { version: latestVersion || status.current_version });
    tone = "success";
  } else if (status.state === "no_release") {
    title = t("No published release");
    detail = t("No stable GitHub release has been published yet.");
    tone = "neutral";
  } else if (status.state === "development") {
    title = t("Development build");
    detail = t("This build does not use a comparable release version.");
    tone = "neutral";
  }
  return (
    <div className="update-result" data-tone={tone} role="status">
      <span className="status-dot" data-state={tone === "success" ? "ok" : tone === "error" ? "warn" : "idle"} />
      <span>
        <strong>{title}</strong>
        <small>{detail}</small>
        {status.stale && <small>{t("Showing the last successful result because GitHub could not be refreshed.")}</small>}
      </span>
    </div>
  );
}
