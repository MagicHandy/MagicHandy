import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { DeviceTransport, StatusSnapshot } from "../api/types";
import { useToast } from "../contexts/ToastContext";
import { TopbarMenu } from "./TopbarMenu";

interface DeviceInfo {
  device_id: string;
  name: string;
  has_linear: boolean;
}

interface HandyConnectionMenuProps {
  snap: StatusSnapshot;
  onRefresh: () => Promise<unknown>;
}

export function HandyConnectionMenu({ snap, onRefresh }: HandyConnectionMenuProps) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [handyKey, setHandyKey] = useState("");
  const [devices, setDevices] = useState<DeviceInfo[]>([]);

  const transport: DeviceTransport =
    snap.device_transport === "handy_cloud" || snap.device_transport === "cloud_rest"
      ? "handy_cloud"
      : "intiface";

  const deviceOk =
    transport === "handy_cloud"
      ? Boolean(snap.handy_connected && snap.device_connected)
      : Boolean(snap.intiface_connected && snap.device_connected);

  const deviceDetail =
    transport === "handy_cloud"
      ? snap.handy_error ?? snap.device_label ?? t("device.handyApi")
      : snap.intiface_error ?? snap.device_label;

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        const handy = (s.handy ?? {}) as Record<string, string>;
        if (handy.connection_key) setHandyKey(handy.connection_key);
      })
      .catch(() => {});
  }, []);

  const wrap = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn();
      notify(ok, "ok");
      await onRefresh();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <TopbarMenu
      label={t("device.handy")}
      connected={deviceOk}
      detail={deviceDetail}
      align="left"
    >
      <div className="menu-panel-section">
        <span className="section-label">{t("device.connection")}</span>
        <label className="field transport-field">
          <span className="hint">{t("device.mode")}</span>
          <select
            className="select-mode"
            value={transport}
            onChange={async (e) => {
              const nextTransport = e.target.value as DeviceTransport;
              try {
                const key = handyKey.trim();
                await api.setDeviceTransport(
                  nextTransport,
                  nextTransport === "handy_cloud" && key ? key : undefined,
                );
                notify(
                  nextTransport === "handy_cloud"
                    ? t("device.modeHandyCloud")
                    : t("device.modeIntiface"),
                  "ok",
                );
                await onRefresh();
              } catch (err) {
                notify(err instanceof Error ? err.message : t("common.error"), "error");
              }
            }}
          >
            <option value="intiface">{t("device.intifaceLocal")}</option>
            <option value="handy_cloud">{t("device.handyKeyApi")}</option>
          </select>
        </label>

        {transport === "handy_cloud" ? (
          <>
            <label className="field">
              <span className="hint">{t("device.connectionKey")}</span>
              <input
                type="password"
                className="mono"
                value={handyKey}
                onChange={(e) => setHandyKey(e.target.value)}
                placeholder={t("device.keyPlaceholder")}
                autoComplete="off"
              />
            </label>
            <div className="btn-row">
              <button
                type="button"
                className="btn btn-sm btn-primary"
                onClick={async () => {
                  const key = handyKey.trim();
                  if (!key && !snap.handy_key_configured) {
                    notify(t("device.keyPlaceholder"), "error");
                    return;
                  }
                  try {
                    await api.setDeviceTransport(
                      "handy_cloud",
                      key || undefined,
                    );
                    await api.connectDevice();
                    notify(t("device.handyConnected"), "ok");
                    await onRefresh();
                  } catch (e) {
                    notify(e instanceof Error ? e.message : t("common.error"), "error");
                  }
                }}
              >
                {t("device.connectApi")}
              </button>
            </div>
            <p className="hint">
              {t("device.cloud")}: <span className="mono">{snap.handy_base_url ?? "—"}</span>
              {!snap.device_connected && ` — ${t("device.saveKeyHint")}`}
            </p>
          </>
        ) : (
          <>
            <div className="btn-row">
              <button
                type="button"
                className="btn btn-sm btn-ghost"
                onClick={() => wrap(() => api.connectDevice(), t("device.connected"))}
              >
                {t("device.intiface")}
              </button>
              <button
                type="button"
                className="btn btn-sm btn-ghost"
                onClick={async () => {
                  try {
                    const res = await api.scanDevices();
                    setDevices(res.devices);
                    notify(t("device.scanResult", { count: res.devices.length }), "ok");
                  } catch (e) {
                    notify(e instanceof Error ? e.message : t("common.error"), "error");
                  }
                }}
              >
                {t("device.scan")}
              </button>
            </div>
            {devices.length > 0 && (
              <ul className="device-list">
                {devices.map((d) => (
                  <li key={d.device_id}>
                    <span>{d.name}</span>
                    <button
                      type="button"
                      className="btn btn-sm btn-ghost"
                      onClick={() =>
                        wrap(() => api.selectDevice(d.device_id), d.name)
                      }
                    >
                      {t("device.use")}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <p className="hint">
              {t("device.intiface")}: <span className="mono">{snap.intiface_url}</span>
              {!snap.device_connected && ` — ${t("device.intifaceHint")}`}
            </p>
          </>
        )}
        <p className="hint mono">{snap.device_label ?? "—"}</p>
      </div>
    </TopbarMenu>
  );
}
