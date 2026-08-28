import { LOCALE_OPTIONS, t, useI18n } from "../i18n";
// The persistent shell: nav rail + status bar + backend banner + routed
// workspace. Stop lives in the rail (always mounted), not here.
import type { ReactNode } from "react";
import { useAppState } from "../state/app-state";
import { VoicePlaybackProvider } from "../state/voice-playback";
import { NavRail } from "./NavRail";
import { StatusBar } from "./StatusBar";
import type { AuthenticationStatus } from "../api/types";

export function AppShell({
  children,
  authenticationLocked = false,
  authenticationStatus = null,
  onLogout,
  onSelectControlIdentity,
}: {
  children: ReactNode;
  authenticationLocked?: boolean;
  authenticationStatus?: AuthenticationStatus | null;
  onLogout?: () => Promise<void>;
  onSelectControlIdentity?: (accountID: string) => Promise<void>;
}) {
  const { backendOnline, state } = useAppState();
  const { loadError, requestedLocale, retry } = useI18n();
  const requestedLanguage = LOCALE_OPTIONS.find((option) => option.value === requestedLocale)?.label
    ?? requestedLocale;
  return (
    <VoicePlaybackProvider>
      <div className="app-shell">
        <NavRail authenticationLocked={authenticationLocked} />
        <StatusBar
          authenticationLocked={authenticationLocked}
          authenticationStatus={authenticationStatus}
          onLogout={onLogout}
          onSelectControlIdentity={onSelectControlIdentity}
        />
        <main className="workspace" id="workspace">
          {!backendOnline && state && (
            <div className="backend-banner" role="alert">
              <strong>{t("Core connection lost.")}</strong>
              <span>{t("Backend-required controls are locked until the core responds.")}</span>
            </div>
          )}
          {loadError && (
            <div className="backend-banner" role="alert">
              <strong>{t("Language resources could not be loaded.")}</strong>
              <span>{requestedLanguage}</span>
              <button className="secondary small" type="button" onClick={retry}>
                {t("Retry")}
              </button>
            </div>
          )}
          {children}
        </main>
      </div>
    </VoicePlaybackProvider>
  );
}
