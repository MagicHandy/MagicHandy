import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { AppStateProvider, ToastProvider } from "./state/app-state";
import { AuthProvider, useAuth } from "./state/auth";
import { I18nProvider } from "./i18n";
import "./styles/tokens.css";
import "./styles/themes.css";
import "./styles/shell.css";
import "./styles/components.css";
import "./styles/setpoint-controls.css";
import "./styles/autopilot.css";
import "./styles/chat.css";
import "./styles/voice.css";
import "./styles/library.css";
import "./styles/media.css";
import "./styles/personas.css";
import "./styles/prompt-inspector.css";
import "./styles/model-manager.css";
import "./styles/setup.css";
import "./styles/update.css";
import "./styles/auth.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

function ApplicationProviders() {
  const { status } = useAuth();
  const accessGranted = Boolean(status && (!status.authentication_required || status.authenticated));
  return (
    <AppStateProvider enabled={accessGranted}>
      <I18nProvider fallbackLocale={status?.ui_locale}>
        <ToastProvider>
          <App />
        </ToastProvider>
      </I18nProvider>
    </AppStateProvider>
  );
}

createRoot(root).render(
  <StrictMode>
    <ErrorBoundary application>
      <AuthProvider>
        <ApplicationProviders />
      </AuthProvider>
    </ErrorBoundary>
  </StrictMode>,
);
