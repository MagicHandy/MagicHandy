import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { AppStateProvider, ToastProvider } from "./state/app-state";
import { I18nProvider } from "./i18n";
import "./styles/tokens.css";
import "./styles/shell.css";
import "./styles/components.css";
import "./styles/chat.css";
import "./styles/voice.css";
import "./styles/library.css";
import "./styles/media.css";
import "./styles/model-manager.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

createRoot(root).render(
  <StrictMode>
    <ErrorBoundary application>
      <AppStateProvider>
        <I18nProvider>
          <ToastProvider>
            <App />
          </ToastProvider>
        </I18nProvider>
      </AppStateProvider>
    </ErrorBoundary>
  </StrictMode>,
);
