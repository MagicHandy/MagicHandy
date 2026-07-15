import { useEffect } from "react";
import { PatternLibraryRoute } from "./routes/PatternLibraryRoute";
import { PresetModesRoute } from "./routes/PresetModesRoute";
import { ChatRoute } from "./routes/ChatRoute";
import { SettingsRoute } from "./routes/SettingsRoute";
import { AppShell } from "./shell/AppShell";
import { routeBase } from "./shell/NavRail";
import { useAppState, useHashRoute } from "./state/app-state";
import { ErrorBoundary } from "./components/ErrorBoundary";

export function App() {
  const route = useHashRoute();
  const base = routeBase(route);
  const { state, startupError, refresh } = useAppState();
  useEffect(() => {
    const workspace = document.getElementById("workspace");
    if (!workspace) return;
    workspace.scrollTop = 0;
    workspace.scrollLeft = 0;
  }, [route]);
  return (
    <AppShell>
      {!state ? (
        <section className="startup-screen" aria-live="polite" aria-busy={!startupError}>
          <div className="startup-mark" aria-hidden="true">MH</div>
          <h1>MagicHandy</h1>
          <p>{startupError || "Starting the core and restoring your workspace..."}</p>
          {!startupError ? <span className="startup-progress" aria-hidden="true" /> : (
            <button type="button" className="btn btn-secondary" onClick={refresh}>Retry core connection</button>
          )}
        </section>
      ) : <ErrorBoundary key={route}>
        {base === "modes" ? (
          <PresetModesRoute />
        ) : base === "library" ? (
          <PatternLibraryRoute />
        ) : base === "settings" ? (
          <SettingsRoute />
        ) : (
          <ChatRoute />
        )}
      </ErrorBoundary>}
    </AppShell>
  );
}
