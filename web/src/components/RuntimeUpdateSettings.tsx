import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { LLMModelManagerSnapshot } from "../api/types";
import { t, translateKnown } from "../i18n";

interface Props {
  automatic: boolean;
  preferenceDisabled: boolean;
  actionDisabled: boolean;
  onAutomaticChange: (automatic: boolean) => void;
}

const message = (error: unknown) => (error instanceof Error ? translateKnown(error.message) : t("Request failed"));
const building = (snapshot: LLMModelManagerSnapshot | null) =>
  snapshot?.runtime_build?.status === "queued" || snapshot?.runtime_build?.status === "building";

// The llama.cpp runtime is pinned by each MagicHandy release. When a release
// moves the pin, this installs the new runtime automatically at startup (the
// default) or waits for Update now; either way it stays verified by SHA-256.
export function RuntimeUpdateSettings({ automatic, preferenceDisabled, actionDisabled, onAutomaticChange }: Props) {
  const [snapshot, setSnapshot] = useState<LLMModelManagerSnapshot | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setSnapshot(await api.llmModels());
    } catch (reason) {
      setError(message(reason));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const active = building(snapshot);
  useEffect(() => {
    if (!active) return undefined;
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => window.clearInterval(timer);
  }, [active, refresh]);

  async function updateNow() {
    if (!snapshot) return;
    setBusy(true);
    setError("");
    try {
      const backend = snapshot.runtime.backend === "cuda" || snapshot.runtime.backend === "cpu" ? snapshot.runtime.backend : "auto";
      const response = await api.buildManagedLlamaRuntime(backend);
      setSnapshot((current) => current ? { ...current, runtime_build: response.build } : current);
    } catch (reason) {
      setError(message(reason));
    } finally {
      setBusy(false);
    }
  }

  const runtime = snapshot?.runtime;
  const outdated = runtime?.state === "outdated";
  let status = t("Checking the llama.cpp runtime...");
  if (active) status = t("Updating the llama.cpp runtime to {version}...", { version: runtime?.expected_version ?? "" });
  else if (runtime?.state === "ready") status = t("Up to date: {version}.", { version: runtime.version ?? runtime.expected_version });
  else if (outdated) status = t("{installed} is installed; this release uses {expected}.", { installed: runtime?.version ?? "", expected: runtime?.expected_version ?? "" });
  else if (runtime?.state === "missing") status = t("Not installed. It installs with a chat model in Setup or Settings > Chat > Model.");
  else if (runtime) status = translateKnown(runtime.message);

  return (
    <div className="update-settings runtime-update-settings">
      <div className="update-version-row">
        <span>{t("llama.cpp runtime")}</span>
        <strong>{runtime?.version || t("Not installed")}</strong>
      </div>
      <p className="hint-block" role="status" aria-live="polite">{status}</p>
      {active && <progress className="runtime-build-progress" aria-label={t("Managed llama.cpp installation in progress")} />}
      {snapshot?.runtime_build?.status === "failed" && <p className="form-status form-status-error">{translateKnown(snapshot.runtime_build.message)}</p>}
      <label className="toggle-line">
        <span className="toggle">
          <input type="checkbox" checked={automatic} disabled={preferenceDisabled} onChange={(event) => onAutomaticChange(event.target.checked)} />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{t("Update the llama.cpp runtime automatically")}</span>
      </label>
      <p className="hint-block">{t("When a MagicHandy update moves to a newer llama.cpp, it installs at the next start with the backend you already use, verified before it replaces the old one. Turn this off to be notified and update when you choose.")}</p>
      {outdated && !active && (
        <div className="actions update-actions">
          <button type="button" className="btn btn-primary" disabled={actionDisabled || busy || !runtime?.build_supported} onClick={() => void updateNow()}>
            {busy ? t("Starting...") : t("Update now")}
          </button>
        </div>
      )}
      {error && <p className="form-status form-status-error" role="alert">{error}</p>}
    </div>
  );
}
