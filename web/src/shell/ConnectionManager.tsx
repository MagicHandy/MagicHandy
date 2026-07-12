import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import conductorHand from "../assets/conductor-hand-v2.png";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, IntifaceTransportSnapshot } from "../api/types";
import { BluetoothBridge, type BluetoothBridgeState } from "../components/BluetoothBridge";
import { IntifacePanel, type IntifaceActivity } from "../components/IntifacePanel";
import { QuickSettings } from "../components/QuickSettings";
import { useAppState, useToast } from "../state/app-state";
import { ChevronUpIcon, CloseIcon, SettingsIcon, WirelessIcon } from "./icons";

type ConnectionPhase = "connected" | "connecting" | "disconnected" | "error";

const SIGNAL_PATHS = [
  "M128 160 Q180 198 232 160",
  "M102 176 Q180 232 258 176",
  "M76 190 Q180 266 284 190",
];

const emptyIntiface: IntifaceTransportSnapshot = {
  dispatch_owner: "intiface",
  address: "",
  status: {
    connected: false,
    scanning: false,
    playback_state: "idle",
    max_ping_time_ms: 0,
    queue_depth: 0,
    devices: [],
  },
  diagnostics: {},
};

const emptyBluetooth: BluetoothBridgeState = {
  connected: false,
  connecting: false,
  status: "disconnected",
  deviceName: "",
};

export function ConnectionManager() {
  const { backendOnline, readOnly, refresh, state } = useAppState();
  const { show } = useToast();
  const [open, setOpen] = useState(false);
  const [cloudBusy, setCloudBusy] = useState(false);
  const [bluetooth, setBluetooth] = useState(emptyBluetooth);
  const [intiface, setIntiface] = useState(state?.intiface_transport ?? emptyIntiface);
  const [intifaceActivity, setIntifaceActivity] = useState<IntifaceActivity | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const restoreFocus = useRef(true);

  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "";
  const keySet = Boolean(state?.settings?.device?.connection_key_set);
  const locked = !backendOnline || readOnly;

  useEffect(() => {
    if (state?.intiface_transport) setIntiface(state.intiface_transport);
  }, [state?.intiface_transport]);

  useEffect(() => {
    if (open) closeRef.current?.focus();
    else if (wasOpen.current && restoreFocus.current) triggerRef.current?.focus();
    wasOpen.current = open;
    restoreFocus.current = true;
  }, [open]);

  const onBluetoothState = useCallback((next: BluetoothBridgeState) => setBluetooth(next), []);
  const onIntifaceSnapshot = useCallback((next: IntifaceTransportSnapshot) => setIntiface(next), []);
  const onIntifaceActivity = useCallback((activity: IntifaceActivity | null) => setIntifaceActivity(activity), []);

  const selectedIntifaceDevice = useMemo(() => {
    const selected = intiface.status.selected_device_index;
    return intiface.status.devices?.find((device) => device.device_index === selected);
  }, [intiface.status.devices, intiface.status.selected_device_index]);

  const provider = owner === "browser_bluetooth"
    ? "Browser Bluetooth"
    : owner === "intiface" ? "Intiface Central" : owner === "cloud_rest" ? "Cloud REST" : "Not configured";
  const deviceName = owner === "browser_bluetooth"
    ? bluetooth.deviceName || "The Handy"
    : selectedIntifaceDevice?.device_name || "The Handy";
  const connecting = cloudBusy || (owner === "browser_bluetooth" && bluetooth.connecting) || (owner === "intiface" && (intifaceActivity === "connecting" || intifaceActivity === "scanning" || intiface.status.scanning));
  const connected = owner === "cloud_rest"
    ? Boolean(state?.cloud_transport?.connected)
    : owner === "browser_bluetooth" ? bluetooth.connected : owner === "intiface"
      ? Boolean(intiface.status.connected && intiface.status.selected_device_index !== undefined)
      : false;
  const hasError = !backendOnline
    || (owner === "browser_bluetooth" && ["error", "unsupported"].includes(bluetooth.status))
    || Boolean(owner === "cloud_rest" && state?.cloud_transport?.last_error)
    || Boolean(owner === "intiface" && typeof intiface.diagnostics.last_error === "string" && intiface.diagnostics.last_error);
  const phase: ConnectionPhase = connecting ? "connecting" : connected ? "connected" : hasError ? "error" : "disconnected";
  const statusText = connectionStatusText({
    backendOnline,
    bluetooth,
    cloudVerified: connected,
    intiface,
    keySet,
    owner,
    phase,
  });

  async function checkCloud() {
    setCloudBusy(true);
    try {
      const result = await api.connectionCheck("cloud");
      show(result.ok ? `The Handy is reachable (${result.latency_ms} ms).` : "The Handy did not report HSP ready.", result.ok ? "info" : "error");
    } catch (error) {
      show(error instanceof Error ? error.message : "Connection check failed.", "error");
    } finally {
      setCloudBusy(false);
      refresh();
    }
  }

  return (
    <div className="connection-manager" data-open={open} data-phase={phase}>
      <section id="connection-manager-panel" className="connection-manager-panel" aria-label="Connection manager" hidden={!open}>
        <header className="connection-manager-head">
          <div>
            <p className="eyebrow">{provider}</p>
            <h2>Connection</h2>
          </div>
          <button ref={closeRef} type="button" className="icon-button" aria-label="Close connection manager" onClick={() => setOpen(false)}>
            <CloseIcon />
          </button>
        </header>

        <ConnectionArtwork phase={phase} />

        <div className="connection-current" aria-live="polite">
          <span className="status-dot" data-state={phase === "connected" ? "ok" : phase === "connecting" ? "pending" : phase === "error" ? "error" : "idle"} aria-hidden="true" />
          <span>
            <strong>{deviceName}</strong>
            <small>{statusText}</small>
          </span>
        </div>

        <div className="connection-provider-actions">
          {owner === "cloud_rest" && (
            <>
              <button type="button" className="btn btn-secondary" disabled={locked || cloudBusy || !keySet} onClick={() => void checkCloud()}>
                <WirelessIcon />
                {cloudBusy ? "Connecting" : connected ? "Check again" : "Check connection"}
              </button>
              {!keySet && <p className="form-status">A Handy connection key is required.</p>}
            </>
          )}
          <BluetoothBridge
            visible={owner === "browser_bluetooth"}
            locked={locked}
            backendOnline={backendOnline}
            initial={state?.bluetooth_bridge as BluetoothBridgeSnapshot | undefined}
            onStateChange={onBluetoothState}
          />
          <IntifacePanel
            visible={owner === "intiface"}
            locked={locked}
            dirty={false}
            initial={state?.intiface_transport}
            onActivityChange={onIntifaceActivity}
            onSnapshotChange={onIntifaceSnapshot}
          />
          <a className="connection-configure" href="#/settings/device" onClick={() => { restoreFocus.current = false; setOpen(false); }}>
            <SettingsIcon />
            Configure device
          </a>
        </div>

        <div className="connection-divider" />
        <div className="connection-limits-head">
          <h3>Limits</h3>
          <span>Applies immediately</span>
        </div>
        <QuickSettings section="limits" />
      </section>

      <button
        ref={triggerRef}
        type="button"
        className="connection-manager-trigger"
        aria-label={`${deviceName} ${statusText}; ${open ? "close" : "open"} connection manager`}
        aria-controls="connection-manager-panel"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="connection-trigger-icon" aria-hidden="true"><WirelessIcon size={20} /></span>
        <span className="connection-trigger-copy">
          <strong>{deviceName}</strong>
          <small>{statusText}</small>
        </span>
        <ChevronUpIcon className="connection-trigger-chevron" />
      </button>
    </div>
  );
}

function ConnectionArtwork({ phase }: { phase: ConnectionPhase }) {
  return (
    <svg
      className="connection-artwork"
      data-phase={phase}
      viewBox="0 0 360 228"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={phase === "connecting" ? "The Handy wireless connection in progress" : "The Handy wireless connection"}
    >
      <image className="connection-hand" href={conductorHand} x="0" y="-85" width="360" height="360" preserveAspectRatio="xMidYMid meet" />
      <g className="connection-signal" aria-hidden="true">
        {SIGNAL_PATHS.map((path, index) => <path key={path} d={path} style={{ "--signal-index": index } as CSSProperties} />)}
      </g>
    </svg>
  );
}

function connectionStatusText(input: {
  backendOnline: boolean;
  bluetooth: BluetoothBridgeState;
  cloudVerified: boolean;
  intiface: IntifaceTransportSnapshot;
  keySet: boolean;
  owner: string;
  phase: ConnectionPhase;
}) {
  if (!input.backendOnline) return "Core offline";
  if (input.phase === "connecting") return input.owner === "intiface" ? "Finding a linear device" : "Connecting";
  if (input.owner === "cloud_rest") {
    if (!input.keySet) return "Connection key required";
    return input.cloudVerified ? "Cloud connection ready" : "Not checked";
  }
  if (input.owner === "browser_bluetooth") {
    return input.bluetooth.connected ? "Bluetooth connected" : input.bluetooth.status === "error" ? "Bluetooth error" : "Bluetooth disconnected";
  }
  if (input.owner === "intiface") {
    if (!input.intiface.status.connected) return "Intiface disconnected";
    return input.intiface.status.selected_device_index === undefined ? "Select a linear actuator" : "Intiface connected";
  }
  return "Choose a dispatch owner";
}
