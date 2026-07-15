// The persistent shell: nav rail + status bar + backend banner + routed
// workspace. Stop lives in the rail (always mounted), not here.
import type { ReactNode } from "react";
import { useAppState } from "../state/app-state";
import { VoicePlaybackProvider } from "../state/voice-playback";
import { NavRail } from "./NavRail";
import { StatusBar } from "./StatusBar";

export function AppShell({ children }: { children: ReactNode }) {
  const { backendOnline, state } = useAppState();
  return (
    <VoicePlaybackProvider>
      <div className="app-shell">
        <NavRail />
        <StatusBar />
        <main className="workspace" id="workspace">
          {!backendOnline && state && (
            <div className="backend-banner" role="alert">
              <strong>Core connection lost.</strong>
              <span>Backend-required controls are locked until the core responds.</span>
            </div>
          )}
          {children}
        </main>
      </div>
    </VoicePlaybackProvider>
  );
}
