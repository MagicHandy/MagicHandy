import { useTranslation } from "react-i18next";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { useToast } from "../hooks/useToast";

interface DeviceInfo {
  device_id: string;
  name: string;
  has_linear: boolean;
}

export function Device() {
  const { t } = useTranslation();
  const { toast, notify } = useToast();
  const [useMock, setUseMock] = useState(true);
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  const [selected, setSelected] = useState("");

  useEffect(() => {
    api.getStatus().then((s) => {
      setUseMock(s.use_mock);
      setSelected(s.device_label);
    });
  }, []);

  const toggleMock = async (enabled: boolean) => {
    try {
      await api.setMockDevice(enabled);
      setUseMock(enabled);
      notify(enabled ? t("device.mockOn") : t("device.mockOff"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const connect = async () => {
    try {
      await api.connectDevice();
      notify(t("device.connected"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const scan = async () => {
    try {
      const res = await api.scanDevices();
      setDevices(res.devices);
      notify(t("device.scanCount", { count: res.devices.length }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const select = async (device_id: string) => {
    try {
      await api.selectDevice(device_id);
      setSelected(device_id);
      notify(t("device.selectedOk"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div>
      <h2>{t("device.title")}</h2>

      <div className="panel">
        <label className="row" style={{ gap: 8 }}>
          <input
            type="checkbox"
            checked={useMock}
            onChange={(e) => toggleMock(e.target.checked)}
          />
          {t("device.useMock")}
        </label>
        <p className="muted">{t("device.selected", { name: selected || "—" })}</p>
      </div>

      <div className="row">
        <button type="button" className="btn" onClick={connect}>
          {t("device.connect")}
        </button>
        <button type="button" className="btn secondary" onClick={scan}>
          {t("device.scan")}
        </button>
      </div>

      {devices.length > 0 && (
        <div className="panel" style={{ marginTop: "1rem" }}>
          <h3 style={{ marginTop: 0 }}>{t("device.foundTitle")}</h3>
          <ul style={{ margin: 0, paddingLeft: "1.2rem" }}>
            {devices.map((d) => (
              <li key={d.device_id} style={{ marginBottom: 8 }}>
                <strong>{d.name}</strong>{" "}
                <span className="muted">
                  {t("device.deviceId", {
                    id: d.device_id,
                    linear: d.has_linear ? t("device.linearYes") : t("device.linearNo"),
                  })}
                </span>
                <button
                  type="button"
                  className="btn secondary"
                  style={{ marginLeft: 8, fontSize: "0.75rem" }}
                  onClick={() => select(d.device_id)}
                >
                  {t("device.select")}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {toast && (
        <div className={`toast ${toast.kind === "error" ? "error" : "ok"}`}>
          {toast.text}
        </div>
      )}
    </div>
  );
}
