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
