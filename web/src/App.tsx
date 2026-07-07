import { PatternLibraryRoute } from "./routes/PatternLibraryRoute";
import { PresetModesRoute } from "./routes/PresetModesRoute";
import { ChatRoute } from "./routes/ChatRoute";
import { SettingsRoute } from "./routes/SettingsRoute";
import { AppShell } from "./shell/AppShell";
import { routeBase } from "./shell/NavRail";
import { useHashRoute } from "./state/app-state";

export function App() {
  const base = routeBase(useHashRoute());
  return (
    <AppShell>
      {base === "modes" ? (
        <PresetModesRoute />
      ) : base === "library" ? (
        <PatternLibraryRoute />
      ) : base === "settings" ? (
        <SettingsRoute />
      ) : (
        <ChatRoute />
      )}
    </AppShell>
  );
}
