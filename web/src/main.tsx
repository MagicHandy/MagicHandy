import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { AppStateProvider, ToastProvider } from "./state/app-state";
import "./styles/tokens.css";
import "./styles/shell.css";
import "./styles/components.css";
import "./styles/library.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

createRoot(root).render(
  <StrictMode>
    <AppStateProvider>
      <ToastProvider>
        <App />
      </ToastProvider>
    </AppStateProvider>
  </StrictMode>,
);
