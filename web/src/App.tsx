import { t, translateKnown, useLocale } from "./i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "./api/client";
import type { ManagedLLMDuplicateSnapshot } from "./api/types";
import { ManagedLLMDuplicateDialog } from "./components/ManagedLLMDuplicateDialog";
import { SetupPromptDialog } from "./components/SetupPromptDialog";
import { PatternLibraryRoute } from "./routes/PatternLibraryRoute";
import { PresetModesRoute } from "./routes/PresetModesRoute";
import { ChatRoute } from "./routes/ChatRoute";
import { PersonasRoute } from "./routes/PersonasRoute";
import { SettingsRoute } from "./routes/SettingsRoute";
import { SetupRoute } from "./routes/SetupRoute";
import { LoginRoute } from "./routes/LoginRoute";
import { VideoRoute } from "./routes/VideoRoute";
import { AppShell } from "./shell/AppShell";
import { routeBase } from "./shell/NavRail";
import { useAppState, useHashRoute } from "./state/app-state";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { normalizeTheme } from "./theme";
import { useAuth } from "./state/auth";
import {LAB_BASE,LabsRoute,legacyLabRoute} from "@labs";

export function App() {
  useLocale();
  const auth = useAuth();
  const route = useHashRoute();
  const base = routeBase(route);
  const { state, startupError, refresh, readOnly } = useAppState();
  const [duplicateRuntime, setDuplicateRuntime] = useState<ManagedLLMDuplicateSnapshot | null>(null);
  const [terminatingDuplicate, setTerminatingDuplicate] = useState(false);
  const [duplicateError, setDuplicateError] = useState("");
  const [setupPromptDismissed, setSetupPromptDismissed] = useState(false);
  const [dismissingSetupPrompt, setDismissingSetupPrompt] = useState(false);
  const [setupPromptError, setSetupPromptError] = useState("");
  const checkedDuplicateConfig = useRef("");
  const llmSettings = state?.settings?.llm;
  const duplicateConfigKey = llmSettings
    ? `${llmSettings.provider}\0${llmSettings.llama_cpp_mode}\0${llmSettings.model}`
    : "";
  const managedLLMSelected = llmSettings?.provider === "llama_cpp" && llmSettings.llama_cpp_mode === "managed";
  const theme = normalizeTheme(state?.settings?.ui?.theme);
  const authenticationLocked = Boolean(auth.status?.authentication_required && !auth.status.authenticated);
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    return () => {
      if (root.dataset.theme === theme) {
        delete root.dataset.theme;
      }
    };
  }, [theme]);
  // A store with no saved settings document at all is a first run, and going
  // straight to the wizard is the onboarding we want. A store that already holds
  // settings but is not marked configured is the other case: an update, or a
  // previous run that left setup part-way. That used to hijack the route on
  // every launch with no way to decline, so it asks instead.
  const setupPending = state?.settings?.ui?.setup_completed === false;
  const setupComplete = state?.settings?.ui?.setup_completed === true;
  const freshStore = (state?.settings_status as { using_defaults?: boolean } | undefined)?.using_defaults === true;
  const setupRoutePath = route.replace(/^#\/?/, "").split("?")[0].replace(/\/+$/, "");
  const explicitSetup = setupRoutePath === "setup/reconfigure";
  const contentBase = setupComplete && base === "setup" && !explicitSetup ? "chat" : base;
  useEffect(()=>{
    const destination=legacyLabRoute(route);
    if(state?.labs_enabled&&destination)window.location.hash=destination;
  },[route,state?.labs_enabled]);
  const askBeforeSetup = setupPending && !freshStore && base !== "setup" && !setupPromptDismissed;
  useEffect(() => {
    if (setupPending && freshStore && base !== "setup") {
      window.location.hash = "#/setup";
    } else if (setupComplete && base === "setup" && !explicitSetup) {
      // An update can revive an existing browser tab whose old hash still
      // points at setup. Only an explicit reconfiguration route may reopen the
      // wizard after completion.
      window.location.hash = "#/chat";
    }
  }, [base, explicitSetup, freshStore, setupComplete, setupPending]);
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

  // Declining has to persist, or the same question returns on every launch,
  // which is the behaviour this replaced. Marking the store configured is what
  // the user is actually saying, and Settings > General still offers "Run setup
  // again". A failure here only leaves the prompt for next time, so the dialog
  // closes either way rather than trapping anyone behind a failed write.
  const declineSetup = async () => {
    if (dismissingSetupPrompt) return;
    setSetupPromptDismissed(true);
    if (readOnly) return;
    setDismissingSetupPrompt(true);
    setSetupPromptError("");
    try {
      await api.completeSetup(true);
      await refresh();
    } catch (error) {
      setSetupPromptError(error instanceof Error ? error.message : t("Setup could not be marked as complete."));
    } finally {
      setDismissingSetupPrompt(false);
    }
  };

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
    <AppShell
      authenticationLocked={authenticationLocked}
      authenticationStatus={auth.status}
      onLogout={auth.logout}
      onSelectControlIdentity={auth.selectControlIdentity}
    >
      {!auth.status ? (
        <section className="startup-screen" aria-live="polite" aria-busy={auth.loading}>
          <div className="startup-mark" aria-hidden="true">MH</div>
          <h1>{t("MagicHandy")}</h1>
          <p>{translateKnown(auth.error || "Checking access…")}</p>
          {auth.error && <button type="button" className="btn btn-secondary" onClick={() => void auth.refresh()}>{t("Retry core connection")}</button>}
          {!auth.error && <span className="startup-progress" aria-hidden="true" />}
        </section>
      ) : authenticationLocked ? (
        <LoginRoute />
      ) : !state ? (
        <section className="startup-screen" aria-live="polite" aria-busy={!startupError}>
          <div className="startup-mark" aria-hidden="true">MH</div>
          <h1>{t("MagicHandy")}</h1>
          <p>{translateKnown(startupError || "Starting the core and restoring your workspace…")}</p>
          {!startupError ? <span className="startup-progress" aria-hidden="true" /> : (
            <button type="button" className="btn btn-secondary" onClick={refresh}>{t("Retry core connection")}</button>
          )}
        </section>
      ) : <ErrorBoundary key={contentBase}>
        {contentBase === "setup" ? (
          <SetupRoute />
        ) : contentBase === "personas" ? (
          <PersonasRoute />
        ) : contentBase === "modes" ? (
          <PresetModesRoute />
        ) : contentBase === "library" ? (
          <PatternLibraryRoute />
        ) : contentBase === "videos" ? (
          <VideoRoute />
        ) : contentBase === "settings" ? (
          <SettingsRoute />
        ) : contentBase === LAB_BASE ? (
          <LabsRoute />
        ) : (
          <ChatRoute />
        )}
      </ErrorBoundary>}
      {askBeforeSetup && (
        <SetupPromptDialog
          pending={dismissingSetupPrompt}
          readOnly={readOnly}
          error={setupPromptError}
          onRunSetup={() => {
            setSetupPromptDismissed(true);
            window.location.hash = "#/setup/reconfigure";
          }}
          onDismiss={() => void declineSetup()}
        />
      )}
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
