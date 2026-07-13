import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent } from "react";
import conductorHand from "../assets/conductor-hand-v2.png";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, IntifaceTransportSnapshot } from "../api/types";
import { BluetoothBridge, type BluetoothBridgeState } from "../components/BluetoothBridge";
import { IntifacePanel, type IntifaceActivity } from "../components/IntifacePanel";
import { QuickSettings } from "../components/QuickSettings";
import { useAppState, useToast } from "../state/app-state";
import { ChevronUpIcon, CloseIcon, RefreshIcon, SettingsIcon, WirelessIcon } from "./icons";

type ConnectionPhase = "connected" | "connecting" | "disconnected" | "error" | "initializing";

const SIGNAL_PATHS = [
  "M145 124 Q180 150 215 124",
  "M123 138 Q180 180 237 138",
  "M100 149 Q180 197 260 149",
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
  const [cloudAttemptFailed, setCloudAttemptFailed] = useState(false);
  // Cloud REST is stateless, so "Disconnect" is a session intent: halt the
  // device and stop treating the last verified check as an active connection
  // until the user reconnects.
  const [cloudSessionEnded, setCloudSessionEnded] = useState(false);
  const [cloudKeyBusy, setCloudKeyBusy] = useState(false);
  const [cloudKey, setCloudKey] = useState("");
  const [bluetooth, setBluetooth] = useState(emptyBluetooth);
  const [intiface, setIntiface] = useState(state?.intiface_transport ?? emptyIntiface);
  const [intifaceActivity, setIntifaceActivity] = useState<IntifaceActivity | null>(null);
  const [intifaceAttemptFailed, setIntifaceAttemptFailed] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const restoreFocus = useRef(true);

  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "";
  const keySet = Boolean(state?.settings?.device?.connection_key_set);
  const bundledApplicationID = state?.settings?.device?.api_application_id_source !== "developer_override";
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
  const onIntifaceAttemptError = useCallback((failed: boolean) => setIntifaceAttemptFailed(failed), []);

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
  // Until the first /api/state poll resolves, `state` is null; show a neutral
  // initializing state rather than a misleading "not configured".
  const initializing = backendOnline && state == null;
  const connecting = cloudBusy || (owner === "browser_bluetooth" && bluetooth.connecting) || (owner === "intiface" && (intifaceActivity === "connecting" || intifaceActivity === "scanning" || intiface.status.scanning));
  const cloudConnected = Boolean(state?.cloud_transport?.connected) && !cloudSessionEnded;
  const connected = owner === "cloud_rest"
    ? cloudConnected
    : owner === "browser_bluetooth" ? bluetooth.connected : owner === "intiface"
      ? Boolean(intiface.status.connected && intiface.status.selected_device_index !== undefined)
      : false;
  const hasError = (owner === "browser_bluetooth" && ["error", "unsupported"].includes(bluetooth.status))
    || (owner === "cloud_rest" && cloudAttemptFailed)
    || (owner === "intiface" && intifaceAttemptFailed);
  const phase: ConnectionPhase = !backendOnline
    ? "disconnected"
    : initializing ? "initializing" : connecting ? "connecting" : connected ? "connected" : hasError ? "error" : "disconnected";
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
    setCloudAttemptFailed(false);
    try {
      const result = await api.connectionCheck("cloud");
      setCloudAttemptFailed(!result.ok);
      if (result.ok) setCloudSessionEnded(false);
      show(result.ok ? `The Handy is reachable (${result.latency_ms} ms).` : "The Handy did not report HSP ready.", result.ok ? "info" : "error");
    } catch (error) {
      setCloudAttemptFailed(true);
      show(error instanceof Error ? error.message : "Connection check failed.", "error");
    } finally {
      setCloudBusy(false);
      refresh();
    }
  }

  async function disconnectCloud() {
    setCloudBusy(true);
    try {
      await api.cloudStop();
      setCloudSessionEnded(true);
      setCloudAttemptFailed(false);
      show("Disconnected from The Handy.");
    } catch (error) {
      show(error instanceof Error ? error.message : "Disconnect failed.", "error");
    } finally {
      setCloudBusy(false);
      refresh();
    }
  }

  async function saveCloudKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextKey = cloudKey.trim();
    if (!nextKey) return;

    setCloudKeyBusy(true);
    try {
      await api.saveConnectionKey(nextKey);
      setCloudKey("");
      show("Handy connection key saved.");
      refresh();
    } catch (error) {
      show(error instanceof Error ? error.message : "Connection key could not be saved.", "error");
    } finally {
      setCloudKeyBusy(false);
    }
  }

  return (
    <div className="connection-manager" data-open={open} data-phase={phase}>
      <section id="connection-manager-panel" className="connection-manager-panel" aria-label="Connection manager" hidden={!open}>
        <header className="connection-manager-head">
          <div className="connection-manager-title">
            <h2>Connection</h2>
            <p>{provider}</p>
          </div>
          <button ref={closeRef} type="button" className="icon-button" aria-label="Close connection manager" onClick={() => setOpen(false)}>
            <CloseIcon />
          </button>
        </header>

        <ConnectionArtwork phase={phase} />

        <div className="connection-current" aria-live="polite">
          <span className="status-dot" data-state={phase === "connected" ? "ok" : phase === "connecting" || phase === "initializing" ? "pending" : phase === "error" ? "error" : "idle"} aria-hidden="true" />
          <span className="connection-current-copy">
            <strong>{deviceName}</strong>
            <small>{statusText}</small>
          </span>
          {owner === "cloud_rest" && connected && (
            <button type="button" className="icon-button connection-retry" aria-label="Re-check connection" title="Re-check connection" disabled={locked || cloudBusy} onClick={() => void checkCloud()}>
              <RefreshIcon size={16} />
            </button>
          )}
        </div>

        <div className="connection-provider-actions">
          {owner === "cloud_rest" && (
            <>
              <form className="connection-key-form" onSubmit={(event) => void saveCloudKey(event)}>
                <div className="connection-key-label">
                  <label htmlFor="connection-manager-key">Handy connection key</label>
                  <span>{keySet ? "Saved" : "Required"}</span>
                </div>
                <div className="connection-key-entry">
                  <input
                    id="connection-manager-key"
                    type="password"
                    autoComplete="off"
                    spellCheck={false}
                    placeholder={keySet ? "Leave blank to keep saved key" : "Paste connection key"}
                    value={cloudKey}
                    disabled={locked || cloudKeyBusy}
                    onChange={(event) => setCloudKey(event.target.value)}
                  />
                  <button type="submit" className="btn btn-secondary" disabled={locked || cloudKeyBusy || !cloudKey.trim()}>
                    {cloudKeyBusy ? "Saving" : "Save key"}
                  </button>
                </div>
              </form>
              {connected ? (
                <button type="button" className="btn btn-secondary" disabled={locked || cloudBusy} onClick={() => void disconnectCloud()}>
                  <WirelessIcon />
                  {cloudBusy ? "Working" : "Disconnect"}
                </button>
              ) : (
                <button type="button" className="btn btn-secondary" disabled={locked || cloudBusy || !keySet} onClick={() => void checkCloud()}>
                  <WirelessIcon />
                  {cloudBusy ? "Connecting" : "Connect"}
                </button>
              )}
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
            onConnectionAttemptError={onIntifaceAttemptError}
            onSnapshotChange={onIntifaceSnapshot}
          />
          <div className="connection-provider-meta">
            {owner === "cloud_rest" && <span>{bundledApplicationID ? "Built-in Handy API v3 ID" : "Developer API v3 ID override"}</span>}
            <a className="connection-configure" href="#/settings/device" onClick={() => { restoreFocus.current = false; setOpen(false); }}>
              <SettingsIcon />
              Configure device
            </a>
          </div>
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
      viewBox="0 0 360 260"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={phase === "connecting" ? "The Handy wireless connection in progress" : "The Handy wireless connection"}
    >
      <image className="connection-hand" href={conductorHand} x="30" y="-77" width="300" height="300" preserveAspectRatio="xMidYMid meet" />
      <g className="connection-signal" aria-hidden="true">
        {SIGNAL_PATHS.map((path, index) => <path key={path} d={path} style={{ "--signal-index": index } as CSSProperties} />)}
      </g>
      <g className="connection-error-mark" data-visible={phase === "error"} aria-hidden="true">
        <path d="m169 139 22 22" />
        <path d="m191 139-22 22" />
      </g>
      <g className="connection-handy" aria-hidden="true">
        <rect className="connection-handy-body" x="146" y="184" width="27" height="70" rx="13.5" />
        <path className="connection-handy-body" d="M180 254v-22.5c0-7.5 6-13.5 13.5-13.5s13.5 6 13.5 13.5V254Z" />
        <circle className="connection-handy-led" cx="159.5" cy="219" r="3" />
        <rect
          className="connection-handy-marker"
          data-state={phase === "connected" ? "connected" : phase === "connecting" ? "connecting" : "disconnected"}
          x="216"
          y="247"
          width="7"
          height="7"
        />
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
  if (input.phase === "initializing") return "Checking…";
  if (input.phase === "connecting") return input.owner === "intiface" ? "Finding a linear device" : "Connecting";
  if (input.phase === "error" && input.owner === "cloud_rest") return "Cloud connection failed";
  if (input.phase === "error" && input.owner === "intiface") return "Intiface connection failed";
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
