import { t, translateKnown, useLocale } from "./i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "./api/client";
import type { ManagedLLMDuplicateSnapshot } from "./api/types";
import { ManagedLLMDuplicateDialog } from "./components/ManagedLLMDuplicateDialog";
import { PatternLibraryRoute } from "./routes/PatternLibraryRoute";
import { PresetModesRoute } from "./routes/PresetModesRoute";
import { ChatRoute } from "./routes/ChatRoute";
import { PersonasRoute } from "./routes/PersonasRoute";
import { SettingsRoute } from "./routes/SettingsRoute";
import { SetupRoute } from "./routes/SetupRoute";
import { VideoRoute } from "./routes/VideoRoute";
import { AppShell } from "./shell/AppShell";
import { routeBase } from "./shell/NavRail";
import { useAppState, useHashRoute } from "./state/app-state";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { normalizeTheme } from "./theme";

export function App() {
  useLocale();
  const route = useHashRoute();
  const base = routeBase(route);
  const { state, startupError, refresh, readOnly } = useAppState();
  const [duplicateRuntime, setDuplicateRuntime] = useState<ManagedLLMDuplicateSnapshot | null>(null);
  const [terminatingDuplicate, setTerminatingDuplicate] = useState(false);
  const [duplicateError, setDuplicateError] = useState("");
  const checkedDuplicateConfig = useRef("");
  const llmSettings = state?.settings?.llm;
  const duplicateConfigKey = llmSettings
    ? `${llmSettings.provider}\0${llmSettings.llama_cpp_mode}\0${llmSettings.model}`
    : "";
  const managedLLMSelected = llmSettings?.provider === "llama_cpp" && llmSettings.llama_cpp_mode === "managed";
  const theme = normalizeTheme(state?.settings?.ui?.theme);
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    return () => {
      if (root.dataset.theme === theme) {
        delete root.dataset.theme;
      }
    };
  }, [theme]);
  useEffect(() => {
    if (state?.settings?.ui?.setup_completed === false && base !== "setup") {
      window.location.hash = "#/setup";
    }
  }, [base, state?.settings?.ui?.setup_completed]);
  useEffect(() => {
    const workspace = document.getElementById("workspace");
    if (!workspace) return;
    workspace.scrollTop = 0;
    workspace.scrollLeft = 0;
  }, [route]);
  useEffect(() => {
    if (!duplicateConfigKey || checkedDuplicateConfig.current === duplicateConfigKey) return;
    if (!managedLLMSelected) {
      checkedDuplicateConfig.current = duplicateConfigKey;
      setDuplicateRuntime(null);
      return;
    }
    let canceled = false;
    let retryTimer: number | undefined;
    const check = async (attempt: number) => {
      try {
        const snapshot = await api.llmDuplicates();
        if (canceled) return;
        checkedDuplicateConfig.current = duplicateConfigKey;
        if (Array.isArray(snapshot.processes) && snapshot.processes.length > 0) {
          setDuplicateError("");
          setDuplicateRuntime(snapshot);
        }
      } catch {
        if (!canceled && attempt < 2) retryTimer = window.setTimeout(() => void check(attempt + 1), 2000);
      }
    };
    void check(0);
    return () => {
      canceled = true;
      window.clearTimeout(retryTimer);
    };
  }, [duplicateConfigKey, managedLLMSelected]);

  const terminateDuplicates = async () => {
    if (!duplicateRuntime || terminatingDuplicate || readOnly) return;
    setTerminatingDuplicate(true);
    setDuplicateError("");
    try {
      const next = await api.terminateLLMDuplicates(duplicateRuntime.processes.map((process) => process.pid));
      if (next.processes.length > 0) setDuplicateRuntime(next);
      else setDuplicateRuntime(null);
      await refresh();
    } catch (error) {
      setDuplicateError(error instanceof Error ? error.message : t("The duplicate process could not be terminated."));
    } finally {
      setTerminatingDuplicate(false);
    }
  };
  return (
    <AppShell>
      {!state ? (
        <section className="startup-screen" aria-live="polite" aria-busy={!startupError}>
          <div className="startup-mark" aria-hidden="true">MH</div>
          <h1>{t("MagicHandy")}</h1>
          <p>{translateKnown(startupError || "Starting the core and restoring your workspace…")}</p>
          {!startupError ? <span className="startup-progress" aria-hidden="true" /> : (
            <button type="button" className="btn btn-secondary" onClick={refresh}>{t("Retry core connection")}</button>
          )}
        </section>
      ) : <ErrorBoundary key={base}>
        {base === "setup" ? (
          <SetupRoute />
        ) : base === "personas" ? (
          <PersonasRoute />
        ) : base === "modes" ? (
          <PresetModesRoute />
        ) : base === "library" ? (
          <PatternLibraryRoute />
        ) : base === "videos" ? (
          <VideoRoute />
        ) : base === "settings" ? (
          <SettingsRoute />
        ) : (
          <ChatRoute />
        )}
      </ErrorBoundary>}
      {duplicateRuntime && (
        <ManagedLLMDuplicateDialog
          snapshot={duplicateRuntime}
          pending={terminatingDuplicate}
          readOnly={readOnly}
          error={duplicateError}
          onCancel={() => setDuplicateRuntime(null)}
          onTerminate={() => void terminateDuplicates()}
        />
      )}
    </AppShell>
  );
}
