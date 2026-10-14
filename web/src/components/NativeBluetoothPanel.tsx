import { useState } from "react";
import { api } from "../api/client";
import type { NativeBluetoothSnapshot } from "../api/native-bluetooth-types";
import { t, translateKnown } from "../i18n";
import { useToast } from "../state/app-state";
import { WirelessIcon } from "../shell/icons";

export function NativeBluetoothPanel({ snapshot, locked, canConfigureHost, onChanged }: {
  snapshot?: NativeBluetoothSnapshot;
  locked: boolean;
  canConfigureHost: boolean;
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState<"connect" | "disconnect" | "forget" | null>(null);
  const { show } = useToast();
  const connected = Boolean(snapshot?.status.connected);
  const connecting = busy === "connect" || Boolean(snapshot?.status.connecting);
  const unavailable = snapshot?.supported === false
    ? t("Native Bluetooth is available on Windows and Linux.")
    : snapshot?.available === false ? t("The native Bluetooth helper is missing. Update MagicHandy to install it.") : "";

  async function run(action: "connect" | "disconnect" | "forget") {
    if (busy) return;
    setBusy(action);
    try {
      if (action === "connect") await api.nativeBluetoothConnect();
      else if (action === "disconnect") await api.nativeBluetoothDisconnect();
      else await api.nativeBluetoothForget();
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Request failed"), "error");
    } finally {
      setBusy(null);
      onChanged();
    }
  }

  return <div className="native-bluetooth-panel">
    <p className="hint-block">{t("Uses Bluetooth on the computer running MagicHandy. Put the Handy in firmware 4 Bluetooth mode before connecting.")}</p>
    {unavailable && <p className="form-status" role="status">{unavailable}</p>}
    <div className="row-actions">
      <button type="button" className={`btn ${connected ? "btn-secondary" : "btn-primary"}`}
        disabled={locked || !canConfigureHost || busy !== null || connecting || Boolean(unavailable) || !snapshot}
        onClick={() => void run(connected ? "disconnect" : "connect")}>
        <WirelessIcon />{connecting ? t("Connecting") : busy === "disconnect" ? t("Disconnecting") : connected ? t("Disconnect") : t("Connect Handy")}
      </button>
      {snapshot?.remembered_device && !connected && <button type="button" className="btn btn-secondary"
        disabled={locked || !canConfigureHost || busy !== null || connecting}
        onClick={() => void run("forget")}>{t("Forget device")}</button>}
    </div>
  </div>;
}
