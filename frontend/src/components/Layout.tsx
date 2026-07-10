import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Outlet } from "react-router-dom";
import { api } from "../api/client";
import {
  isOllamaProvider,
  llmProviderFromSnap,
} from "../lib/llmStatus";
import { Sidebar } from "./Sidebar";
import { ShellTopbar } from "./ShellTopbar";
import { ControllerBanner } from "./ControllerBanner";
import { ErrorBoundary } from "./ErrorBoundary";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";
import { useGlobalEscStop } from "../hooks/useGlobalEscStop";
import { UI_VERSION } from "../version";

export function Layout() {
  const { t } = useTranslation();
  const { snap, controller, error, refresh } = useStatus();
  const { notify } = useToast();
  const [buildLabel, setBuildLabel] = useState<string | null>(null);

  useEffect(() => {
    fetch("/api/system/build-info")
      .then((r) => (r.ok ? r.json() : null))
      .then((info) => {
        if (!info?.ui_version) return;
        const stamp = info.built_at
          ? String(info.built_at).slice(0, 19).replace("T", " ")
          : "?";
        setBuildLabel(`${info.ui_version ?? UI_VERSION} · ${stamp}`);
      })
      .catch(() => {
        setBuildLabel(UI_VERSION);
      });
  }, []);

  const recheckOllama = async () => {
    try {
      const r = await api.pingOllama();
      await refresh();
      const connected = Boolean(r.llm_connected ?? r.ollama_connected);
      const provider = r.llm_provider ?? r.provider ?? llmProviderFromSnap(snap ?? {});
      const err = String(r.llm_error ?? r.ollama_error ?? r.error ?? "?");
      if (connected) {
        notify(
          isOllamaProvider(provider)
            ? t("layout.ollama.connected")
            : t("layout.llamaCpp.connected"),
          "ok",
        );
      } else {
        notify(
          isOllamaProvider(provider)
            ? t("layout.ollama.off", { error: err })
            : t("layout.llamaCpp.off", { error: err }),
          "error",
        );
      }
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const onStop = async () => {
    try {
      await api.emergencyStop("ui_stop");
      await api.stopMotion().catch(() => {});
    } catch {
      /* footer poll will update */
    }
  };

  useGlobalEscStop(onStop);

  const emergency = snap?.emergency_stop;

  return (
    <div className="shell shell--v12">
      <div className="shell-main">
        {snap && !error ? (
          <ShellTopbar
            snap={snap}
            controller={controller}
            emergency={emergency}
            onStop={onStop}
            onRecheckOllama={recheckOllama}
            onRefresh={refresh}
          />
        ) : (
          <header className="topbar topbar--v2 topbar--offline">
            <span className="hint">{t("layout.waitingApi")}</span>
            <button
              type="button"
              className={`btn-stop topbar-stop${emergency ? " active" : ""}`}
              onClick={onStop}
              title={t("layout.emergencyStop")}
            >
              {t("common.stop")}
            </button>
          </header>
        )}

        {snap?.intiface_reconnecting && (
          <div className="alert alert-warn reconnect-banner">
            {t("layout.reconnectingIntiface")}
          </div>
        )}

        <ControllerBanner controller={controller} />

        <main className="content">
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </main>

        <footer className={`statusbar${emergency ? " emergency" : ""}`}>
          <span>
            {snap?.footer_status ?? error ?? "…"}
            {buildLabel && (
              <span className="statusbar-build" title={t("layout.uiVersion")}>
                {" "}
                · {buildLabel}
              </span>
            )}
          </span>
          {emergency && (
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => api.clearEmergencyStop()}
            >
              {t("layout.clearStop")}
            </button>
          )}
        </footer>
      </div>

      <Sidebar snap={snap} error={error} />

      <button
        type="button"
        className={`btn-stop mobile-stop-fab${emergency ? " active" : ""}`}
        onClick={onStop}
        title={t("layout.emergencyStop")}
        aria-label={t("layout.emergencyStop")}
      >
        {t("common.stop")}
      </button>
    </div>
  );
}
