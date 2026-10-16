// The shell's device runtime. It keeps the browser Bluetooth session mounted
// on every route, tracks this tab's in-flight device requests, and polls the
// live levels faster while a panel or device page is watching. Device state
// itself stays backend-owned: these are snapshots, never a parallel model.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import { configuredDevices, playbackDevices } from "../api/devices";
import type { CoyoteTransportSnapshot, DeviceId, IntifaceTransportSnapshot } from "../api/types";
import { BluetoothBridge, type BluetoothBridgeHandle, type BluetoothBridgeState } from "../components/BluetoothBridge";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";
import type { DeviceContext } from "./types";

// Levels change faster than the app-state poll while someone watches them.
const WATCH_POLL_MS = 500;

export const EMPTY_INTIFACE: IntifaceTransportSnapshot = {
  dispatch_owner: "intiface",
  address: "",
  status: { connected: false, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices: [] },
  diagnostics: {},
};

const EMPTY_BLUETOOTH: BluetoothBridgeState = { connected: false, connecting: false, status: "disconnected", deviceName: "", supported: false };

interface DeviceRuntime {
  busy: Partial<Record<DeviceId, string>>;
  failed: Partial<Record<DeviceId, boolean>>;
  setFailed: (device: DeviceId, failed: boolean) => void;
  run: DeviceContext["run"];
  coyote?: CoyoteTransportSnapshot;
  setCoyote: (snapshot: CoyoteTransportSnapshot) => void;
  intiface?: IntifaceTransportSnapshot;
  setIntiface: (snapshot: IntifaceTransportSnapshot) => void;
  bluetooth: BluetoothBridgeState;
  browserBluetooth: DeviceContext["browserBluetooth"];
  watch: () => () => void;
  setBluetoothDetails: (target: HTMLElement | null) => void;
}

const DeviceRuntimeContext = createContext<DeviceRuntime | null>(null);

export function DeviceRuntimeProvider({ children }: { children: ReactNode }) {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const [busy, setBusy] = useState<Partial<Record<DeviceId, string>>>({});
  const [failed, setFailedMap] = useState<Partial<Record<DeviceId, boolean>>>({});
  const pending = useRef(new Set<DeviceId>());
  const [coyote, setCoyote] = useState(state?.coyote_transport);
  const [intiface, setIntiface] = useState(state?.intiface_transport);
  const [bluetooth, setBluetooth] = useState(EMPTY_BLUETOOTH);
  const [watchers, setWatchers] = useState(0);
  const [bluetoothDetails, setBluetoothDetails] = useState<HTMLElement | null>(null);
  const bridge = useRef<BluetoothBridgeHandle>(null);
  const configured = configuredDevices(state?.settings?.device);
  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "";
  const watching = watchers > 0;
  const coyoteConnected = configured.includes("coyote") && Boolean(coyote?.status.connected);
  const vibratorLeased = configured.includes("vibrator") && Boolean(intiface?.status.connected && intiface.leases?.includes("vibrator"));
  const usesIntiface = configured.includes("vibrator") || (configured.includes("handy") && owner === "intiface");

  useEffect(() => { if (state?.coyote_transport) setCoyote(state.coyote_transport); }, [state?.coyote_transport]);
  useEffect(() => { if (state?.intiface_transport) setIntiface(state.intiface_transport); }, [state?.intiface_transport]);
  // A failed attempt belongs to the method it was made with.
  useEffect(() => setFailedMap((current) => (current.handy ? { ...current, handy: false } : current)), [owner]);

  // A watcher opening shows current device lists at once, not a poll later.
  useEffect(() => {
    if (!watching || !backendOnline || !usesIntiface) return;
    void api.intifaceStatus().then(setIntiface).catch(() => undefined);
  }, [watching, backendOnline, usesIntiface]);
  useEffect(() => {
    if (!watching || !coyoteConnected) return;
    const timer = window.setInterval(() => void api.coyoteStatus().then(setCoyote).catch(() => undefined), WATCH_POLL_MS);
    return () => window.clearInterval(timer);
  }, [watching, coyoteConnected]);
  useEffect(() => {
    if (!watching || !vibratorLeased) return;
    const timer = window.setInterval(() => void api.intifaceStatus().then(setIntiface).catch(() => undefined), WATCH_POLL_MS);
    return () => window.clearInterval(timer);
  }, [watching, vibratorLeased]);

  // A connection attempt's outcome stays on its row until the next attempt.
  const run = useCallback<DeviceContext["run"]>(async (device, action, request) => {
    if (pending.current.has(device)) return false;
    pending.current.add(device);
    setBusy((current) => ({ ...current, [device]: action }));
    const connecting = action === "connect";
    if (connecting) setFailedMap((current) => ({ ...current, [device]: false }));
    try {
      const success = await request();
      if (success) show(success);
      return true;
    } catch (error) {
      if (connecting) setFailedMap((current) => ({ ...current, [device]: true }));
      show(error instanceof Error ? translateKnown(error.message) : t("The device request failed."), "error");
      return false;
    } finally {
      pending.current.delete(device);
      setBusy((current) => {
        const next = { ...current };
        delete next[device];
        return next;
      });
      void refresh();
    }
  }, [refresh, show]);

  const setFailed = useCallback((device: DeviceId, value: boolean) => setFailedMap((current) => ({ ...current, [device]: value })), []);
  const watch = useCallback(() => {
    setWatchers((count) => count + 1);
    return () => setWatchers((count) => count - 1);
  }, []);
  const browserBluetooth = useMemo(() => ({
    connect: () => bridge.current?.connect(),
    disconnect: () => bridge.current?.disconnect(),
  }), []);

  const runtime = useMemo<DeviceRuntime>(() => ({
    busy, failed, setFailed, run, coyote, setCoyote, intiface, setIntiface, bluetooth, browserBluetooth, watch, setBluetoothDetails,
  }), [busy, failed, setFailed, run, coyote, intiface, bluetooth, browserBluetooth, watch]);

  return (
    <DeviceRuntimeContext.Provider value={runtime}>
      {children}
      {/* One instance owns the browser's Bluetooth session on every route;
          the Handy's page shows its details through a portal. */}
      <BluetoothBridge
        ref={bridge}
        visible={configured.includes("handy") && owner === "browser_bluetooth"}
        locked={!backendOnline || readOnly}
        backendOnline={backendOnline}
        canConfigureHost={state?.capabilities?.configure_host !== false}
        initial={state?.bluetooth_bridge}
        onStateChange={setBluetooth}
        detailsTarget={bluetoothDetails}
        showActions={false}
      />
    </DeviceRuntimeContext.Provider>
  );
}

/** Polls the live levels faster while the caller is shown. */
export function useDeviceWatch(active = true) {
  const runtime = useContext(DeviceRuntimeContext);
  const watch = runtime?.watch;
  useEffect(() => (active && watch ? watch() : undefined), [active, watch]);
}

/** Where the Handy's page wants the browser Bluetooth details. */
export function useBluetoothDetailsTarget(): (target: HTMLElement | null) => void {
  return useContext(DeviceRuntimeContext)?.setBluetoothDetails ?? noop;
}

function noop() {}

/** The live device context for one render. Without the shell's provider (a
 * component rendered alone) it reads the backend snapshots directly. */
export function useDeviceContext(): DeviceContext {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const runtime = useContext(DeviceRuntimeContext);
  const detached = useCallback<DeviceContext["run"]>(async (_device, _action, request) => {
    try {
      const success = await request();
      if (success) show(success);
      return true;
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("The device request failed."), "error");
      return false;
    } finally {
      void refresh();
    }
  }, [refresh, show]);
  const send = useCallback((request: () => Promise<unknown>) => {
    void request().catch((error: unknown) => show(error instanceof Error ? translateKnown(error.message) : t("The device request failed."), "error"));
  }, [show]);
  const engine = state?.motion?.engine;
  const linked = engine?.linked_session;
  const settings = state?.settings;
  return {
    state,
    settings,
    backendOnline,
    readOnly,
    locked: !backendOnline || readOnly,
    canConfigureHost: state?.capabilities?.configure_host !== false,
    configured: configuredDevices(settings?.device),
    playing: playbackDevices(settings?.device),
    engineActive: Boolean(engine?.running || engine?.starting || engine?.completing),
    retired: linked?.status === "degraded" ? linked.retired_devices.filter(isDevice) : [],
    stopSequence: state?.stop_sequence,
    intiface: runtime?.intiface ?? state?.intiface_transport ?? EMPTY_INTIFACE,
    coyote: runtime?.coyote ?? state?.coyote_transport,
    native: state?.native_bluetooth_transport,
    bluetooth: runtime?.bluetooth ?? EMPTY_BLUETOOTH,
    busy: runtime?.busy ?? {},
    failed: runtime?.failed ?? {},
    run: runtime?.run ?? detached,
    send,
    setFailed: runtime?.setFailed ?? noop,
    setCoyote: runtime?.setCoyote ?? noop,
    setIntiface: runtime?.setIntiface ?? noop,
    browserBluetooth: runtime?.browserBluetooth ?? { connect: noop, disconnect: noop },
  };
}

function isDevice(value: string): value is DeviceId {
  return value === "handy" || value === "coyote" || value === "vibrator";
}
