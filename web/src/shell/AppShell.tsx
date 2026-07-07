// The persistent shell: nav rail + status bar + backend banner + routed
// workspace. Stop lives in the rail (always mounted), not here.
import type { ReactNode } from "react";
import { useAppState } from "../state/app-state";
import { NavRail } from "./NavRail";
import { StatusBar } from "./StatusBar";

export function AppShell({ children }: { children: ReactNode }) {
  const { backendOnline } = useAppState();
  return (
    <div className="app-shell">
      <NavRail />
      <StatusBar />
      <main className="workspace" id="workspace">
        {!backendOnline && (
          <div className="backend-banner" role="alert">
            <strong>Core connection lost.</strong>
            <span>Backend-required controls are locked until the core responds.</span>
          </div>
        )}
        {children}
      </main>
    </div>
  );
}
