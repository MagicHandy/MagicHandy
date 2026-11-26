import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { AuthProvider } from "./state/auth";
import { ApplicationProviders } from "./state/ApplicationProviders";
import "./styles/tokens.css";
import "./styles/themes.css";
import "./styles/shell.css";
import "./styles/components.css";
import "./styles/visualizer.css";
import "./styles/setpoint-controls.css";
// Stylesheets whose classes appear on more than one route load here, in their
// long-standing cascade order. Route-only sheets (freestyle, video-library)
// load with their route. scripts/check-route-styles.mjs enforces the split.
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
import "./styles/settings-navigation.css";
import "./styles/network-setup.css";
import "./styles/cloud-motion.css";
import "./styles/form-rows.css";
import "./styles/notices.css";
import "./styles/settings-form.css";
import "./styles/devices.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

createRoot(root).render(
  <StrictMode>
    <ErrorBoundary application>
      <AuthProvider>
        <ApplicationProviders />
      </AuthProvider>
    </ErrorBoundary>
  </StrictMode>,
);
