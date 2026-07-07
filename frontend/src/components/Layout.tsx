import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Outlet } from "react-router-dom";
import { api } from "../api/client";
import { Sidebar } from "./Sidebar";
import { ShellTopbar } from "./ShellTopbar";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";
import { UI_VERSION } from "../version";

export function Layout() {
  const { t } = useTranslation();
  const { snap, error, refresh } = useStatus();
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
      notify(
        r.ollama_connected
          ? t("layout.ollama.connected")
          : t("layout.ollama.off", { error: r.ollama_error ?? r.error ?? "?" }),
        r.ollama_connected ? "ok" : "error",
      );
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const onStop = async () => {
    try {
      await api.emergencyStop("ui_stop");
    } catch {
      /* footer poll will update */
    }
  };

  const emergency = snap?.emergency_stop;

  return (
    <div className="shell shell--pro">
      <Sidebar snap={snap} error={error} />

      <div className="main-col">
        {snap && !error ? (
          <ShellTopbar
            snap={snap}
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

        <main className="content">
          <Outlet />
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
    </div>
  );
}
