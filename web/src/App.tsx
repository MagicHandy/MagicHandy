import { useEffect } from "react";
import { PatternLibraryRoute } from "./routes/PatternLibraryRoute";
import { PresetModesRoute } from "./routes/PresetModesRoute";
import { ChatRoute } from "./routes/ChatRoute";
import { SettingsRoute } from "./routes/SettingsRoute";
import { AppShell } from "./shell/AppShell";
import { routeBase } from "./shell/NavRail";
import { useHashRoute } from "./state/app-state";
import { ErrorBoundary } from "./components/ErrorBoundary";

export function App() {
  const route = useHashRoute();
  const base = routeBase(route);
  useEffect(() => {
    const workspace = document.getElementById("workspace");
    if (!workspace) return;
    workspace.scrollTop = 0;
    workspace.scrollLeft = 0;
  }, [route]);
  return (
    <AppShell>
      <ErrorBoundary key={route}>
        {base === "modes" ? (
          <PresetModesRoute />
        ) : base === "library" ? (
          <PatternLibraryRoute />
        ) : base === "settings" ? (
          <SettingsRoute />
        ) : (
          <ChatRoute />
        )}
      </ErrorBoundary>
    </AppShell>
  );
}
