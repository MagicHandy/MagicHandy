import { t, translateKnown } from "../i18n";
import { useCallback, useEffect, useId, useMemo, useRef, useState, type CSSProperties, type FormEvent } from "react";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, DeviceOutput, IntifaceTransportSnapshot } from "../api/types";
import { BluetoothBridge, type BluetoothBridgeState } from "../components/BluetoothBridge";
import { CoyotePanel, coyoteStatusText, coyoteStrengthText } from "../components/CoyotePanel";
import { IntifacePanel, type IntifaceActivity } from "../components/IntifacePanel";
import { useMenu } from "../components/useMenu";
import { QuickSettings } from "../components/QuickSettings";
import { useAppState, useToast } from "../state/app-state";
import { ConnectionMethodPicker, handyConnectionMethods } from "./ConnectionMethodPicker";
import { ChevronUpIcon, CloseIcon, PlusIcon, RefreshIcon, SettingsIcon, WirelessIcon } from "./icons";

type ConnectionPhase = "connected" | "connecting" | "disconnected" | "error" | "initializing";

// Three concentric arcs radiate upward from the device glyph, inner to outer,
// in the flat status strip at the top of the connection panel.
const SIGNAL_PATHS = [
  "M166 34 Q176.5 25 187 34",
  "M157 25 Q176.5 9 196 25",
  "M148 16 Q176.5 -7 205 16",
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

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  restoreFocusOnClose?: boolean;
}

export function ConnectionManager({ open, onOpenChange, restoreFocusOnClose = true }: Props) {
  const { backendOnline, readOnly, refresh, state } = useAppState();
  const { show } = useToast();
  const [cloudAction, setCloudAction] = useState<"check" | "connect" | "disconnect" | null>(null);
  const cloudBusy = cloudAction !== null;
  const [cloudAttemptFailed, setCloudAttemptFailed] = useState(false);
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
  const titleId = useId();
  const handyTitleId = useId();
  const coyoteTitleId = useId();
  const addRef = useRef<HTMLButtonElement>(null);
  const focusAfterRemoval = useRef(false);

  const owner = state?.settings?.device?.hsp_dispatch_owner ?? "";
  const output: DeviceOutput = normalizeOutput(state?.settings?.device?.output);
  const configured = state?.settings?.device?.configured_devices;
  const usesHandy = configured ? configured.includes("handy") : output === "handy" || output === "handy_coyote";
  const usesCoyote = configured ? configured.includes("coyote") : output === "coyote" || output === "handy_coyote";
  const handyIncluded = output === "handy" || output === "handy_coyote";
  const coyoteIncluded = output === "coyote" || output === "handy_coyote";
  const coyote = state?.coyote_transport;
  const linkedSession = state?.motion?.engine?.linked_session;
  const canConfigureHost = state?.capabilities?.configure_host !== false;
  // Device and method changes keep their controls focusable while a request
  // runs; the refs drop repeated choices instead of disabling the control.
  const [outputBusy, setOutputBusy] = useState(false);
  const [methodBusy, setMethodBusy] = useState(false);
  const outputPending = useRef(false);
  const methodPending = useRef(false);
  const keySet = Boolean(state?.settings?.device?.connection_key_set);
  const bundledApplicationID = state?.settings?.device?.api_application_id_source !== "developer_override";
  const locked = !backendOnline || readOnly;
  const advertisedMethods = state?.settings?.options?.hsp_dispatch_owners;
  const methods = handyConnectionMethods().filter((method) => !advertisedMethods?.length || advertisedMethods.includes(method.value));

  useEffect(() => {
    if (state?.intiface_transport) setIntiface(state.intiface_transport);
  }, [state?.intiface_transport]);

  // A failed attempt belongs to the method it was made with.
  useEffect(() => {
    setCloudAttemptFailed(false);
    setIntifaceAttemptFailed(false);
  }, [owner]);

  useEffect(() => {
    if (open) closeRef.current?.focus();
    else if (wasOpen.current && restoreFocus.current && restoreFocusOnClose) triggerRef.current?.focus();
    wasOpen.current = open;
    restoreFocus.current = true;
  }, [open, restoreFocusOnClose]);

  useEffect(() => {
    const showConnections = () => onOpenChange(true);
    window.addEventListener("magichandy:open-connections", showConnections);
    return () => window.removeEventListener("magichandy:open-connections", showConnections);
  }, [onOpenChange]);

  const onBluetoothState = useCallback((next: BluetoothBridgeState) => setBluetooth(next), []);
  const onIntifaceSnapshot = useCallback((next: IntifaceTransportSnapshot) => setIntiface(next), []);
  const onIntifaceActivity = useCallback((activity: IntifaceActivity | null) => setIntifaceActivity(activity), []);
  const onIntifaceAttemptError = useCallback((failed: boolean) => setIntifaceAttemptFailed(failed), []);

  const selectedIntifaceDevice = useMemo(() => {
    const selected = intiface.status.selected_device_index;
    return intiface.status.devices?.find((device) => device.device_index === selected);
  }, [intiface.status.devices, intiface.status.selected_device_index]);

  const initializing = backendOnline && state == null;
  // The title names how MagicHandy reaches the device: the Handy's method, or
  // the Coyote's own Bluetooth link when the Coyote is the only output.
  const methodLabel = !usesHandy && !initializing
    ? t("Coyote Bluetooth")
    : methods.find((method) => method.value === owner)?.label ?? translateKnown(connectionProviderName(owner, initializing));
  const canChooseDevice = !locked && canConfigureHost && !initializing;
  const selectionBusy = outputBusy || methodBusy;
  const canAdd = canChooseDevice && !(usesHandy && usesCoyote);
  const addMenu = useMenu({ disabled: !open || !canAdd || selectionBusy });
  const lockReason = !backendOnline
    ? t("Core offline.")
    : readOnly ? t("Read-only client.") : canConfigureHost ? "" : t("Host settings and diagnostics are managed by an administrator.");
  const coyoteSafetyAcknowledged = Boolean(state?.settings?.estim?.safety_acknowledged);
  const handyName = owner === "browser_bluetooth"
    ? bluetooth.deviceName || "The Handy"
    : selectedIntifaceDevice?.device_name || "The Handy";
  const deviceName = !usesHandy && !usesCoyote ? t("Connections") : !usesHandy ? t("Coyote") : usesCoyote ? t("{device} + Coyote", { device: handyName }) : handyName;
  const intifaceReady = Boolean(intiface.status.connected && intiface.status.selected_device_index !== undefined);
  // An Intiface scan runs until it is stopped, so a scan only means "still
  // finding a device" while no actuator has been chosen.
  const intifaceSearching = !intifaceReady && (intifaceActivity === "scanning" || intiface.status.scanning);
  const connecting = cloudAction === "check" || cloudAction === "connect" || (owner === "browser_bluetooth" && bluetooth.connecting) || (owner === "intiface" && (intifaceActivity === "connecting" || intifaceSearching));
  const connected = owner === "cloud_rest"
    ? Boolean(state?.cloud_transport?.connected)
    : owner === "browser_bluetooth" ? bluetooth.connected : owner === "intiface"
      ? intifaceReady
      : false;
  const hasError = (owner === "browser_bluetooth" && ["error", "unsupported"].includes(bluetooth.status))
    || (owner === "cloud_rest" && cloudAttemptFailed)
    || (owner === "intiface" && intifaceAttemptFailed);
  const handyPhase: ConnectionPhase = !backendOnline
    ? "disconnected"
    : initializing ? "initializing" : connecting ? "connecting" : connected ? "connected" : hasError ? "error" : "disconnected";
  const coyotePhase: ConnectionPhase = !backendOnline
    ? "disconnected"
    : initializing ? "initializing" : coyote?.status.connecting ? "connecting" : coyote?.status.connected ? "connected" : "disconnected";
  const phase = !usesHandy ? coyotePhase : handyPhase;
  const handyStatus = connectionStatusText({
    backendOnline,
    bluetooth,
    cloudVerified: connected,
    intiface,
    keySet,
    owner,
    phase: handyPhase,
  });
  const coyoteLine = [translateKnown(coyoteStatusText(coyote?.status)), coyoteStrengthText(coyote?.status)].filter(Boolean).join(" · ");
  const statusText = !backendOnline
    ? translateKnown("Core offline")
    : !usesHandy && !usesCoyote ? t("Chat only")
    : !usesHandy
      ? coyoteLine
      : usesCoyote ? `${translateKnown(handyStatus)} · ${coyoteLine}` : translateKnown(handyStatus);

  useEffect(() => {
    if (focusAfterRemoval.current && !(usesHandy && usesCoyote)) {
      focusAfterRemoval.current = false;
      addRef.current?.focus();
    }
  }, [usesHandy, usesCoyote]);

  async function changeConnection(device: "handy" | "coyote", action: "add" | "remove" | "include" | "exclude") {
    if (!canChooseDevice || outputPending.current || methodPending.current) return;
    outputPending.current = true;
    setOutputBusy(true);
    try {
      const result = await api.changeDeviceConnection(device, action);
      if ("warning" in result && result.warning) show(t("the setting was saved, but the device runtime could not be updated"), "warning");
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("The device could not be changed."), "error");
    } finally {
      outputPending.current = false;
      setOutputBusy(false);
      refresh();
    }
  }

  // The Handy's method saves through the same preferences endpoint Setup uses.
  // The backend stops motion and closes the previous session; the title shows
  // the new method only once the refreshed settings report it. A warning means
  // the method was saved but stopping or updating the device did not finish.
  async function changeMethod(next: string) {
    if (!canChooseDevice || next === owner || methodPending.current || outputPending.current) return;
    methodPending.current = true;
    setMethodBusy(true);
    try {
      const result = await api.saveSetupPreferences({ device_owner: next });
      if ("warning" in result && result.warning) show(t("the setting was saved, but the device runtime could not be updated"), "warning");
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("The device could not be changed."), "error");
    } finally {
      methodPending.current = false;
      setMethodBusy(false);
      refresh();
    }
  }

  async function probeCloud(action: "check" | "connect") {
    setCloudAction(action);
    setCloudAttemptFailed(false);
    try {
      const result = action === "connect" ? await api.cloudConnect() : await api.connectionCheck("cloud");
      setCloudAttemptFailed(!result.ok);
      show(
        result.ok
          ? t("The Handy is reachable ({latency} ms).", { latency: result.latency_ms })
          : result.message?.trim() || t("The Handy responded, but HSP is unavailable for its current device state."),
        result.ok ? "info" : "error",
      );
    } catch (error) {
      setCloudAttemptFailed(true);
      show(error instanceof Error ? translateKnown(error.message) : action === "connect" ? t("Connection attempt failed.") : t("Connection check failed."), "error");
    } finally {
      setCloudAction(null);
      refresh();
    }
  }

  async function disconnectCloud() {
    setCloudAction("disconnect");
    setCloudAttemptFailed(false);
    try {
      const result = await api.cloudDisconnect();
      if (result.warning) show(translateKnown(result.warning), "error");
      else show(t("MagicHandy released control of The Handy. It can now be used by another app."));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("The Handy could not be disconnected."), "error");
    } finally {
      setCloudAction(null);
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
      show(t("Handy connection key saved."));
      refresh();
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Connection key could not be saved."), "error");
    } finally {
      setCloudKeyBusy(false);
    }
  }

  return (
    <div className="connection-manager" data-open={open} data-phase={phase}>
      <section id="connection-manager-panel" className="connection-manager-panel" aria-label={t("Connection manager")} hidden={!open}>
        <header className="connection-manager-head">
          <div className="connection-manager-title">
            <h2 id={titleId}>{t("Connections")}</h2>
          </div>
          <div className="connection-manager-head-actions">
            <div className="connection-add">
              <button ref={addRef} type="button" className="icon-button connection-add-trigger"
                aria-label={t("Add connection")} aria-haspopup="menu" aria-expanded={addMenu.open} aria-controls={addMenu.open ? addMenu.id : undefined}
                aria-disabled={!canAdd || selectionBusy} aria-busy={selectionBusy || undefined}
                title={usesHandy && usesCoyote ? t("The Handy and Coyote are already added.") : t("Add connection")}
                onClick={(event) => { if (addMenu.open) addMenu.close(); else if (canAdd && !selectionBusy) addMenu.show(event.currentTarget, undefined); }}
                onKeyDown={(event) => { if (canAdd && !selectionBusy) addMenu.onTriggerKeyDown(event, undefined); }}>
                <PlusIcon size={16} />
              </button>
              {addMenu.open && <div {...addMenu.menuProps} className="connection-method-menu" aria-label={t("Add connection")}>
                {(["handy", "coyote"] as const).filter((device) => device === "handy" ? !usesHandy : !usesCoyote).map((device) => (
                  <button key={device} type="button" role="menuitem" className="connection-method-option" onClick={() => { addMenu.close(); void changeConnection(device, "add"); }}>
                    <span className="connection-method-option-copy">
                      <strong>{device === "coyote" ? t("Coyote") : t("The Handy")}</strong>
                      <small>{device === "coyote" ? t("Coyote Bluetooth") : methods.find((method) => method.value === owner)?.label ?? t("Not configured")}</small>
                    </span><PlusIcon size={16} />
                  </button>
                ))}
                <p className="connection-method-hint">{t("Adds a saved connection. Connect it and choose Use in playback when ready.")}</p>
              </div>}
            </div>

            <a
              className="icon-button"
              href="#/settings/device"
              aria-label={t("Configure device")}
              title={t("Configure device")}
              onClick={() => { restoreFocus.current = false; onOpenChange(false); }}
            >
              <SettingsIcon />
            </a>
            <button ref={closeRef} type="button" className="icon-button" aria-label={t("Close connection manager")} onClick={() => onOpenChange(false)}>
              <CloseIcon />
            </button>
          </div>
        </header>

        {lockReason && <p className="form-status connection-lock-reason">{lockReason}</p>}
        {!usesHandy && !usesCoyote && <div className="connection-section connection-empty"><strong>{t("Chat only")}</strong><p className="hint-block">{t("Add a connection with + when you want device playback.")}</p></div>}
        {output === "none" && (usesHandy || usesCoyote) && <p className="connection-playback-note">{t("No devices selected for playback.")}</p>}

        {linkedSession?.status === "failed" && <p className="connection-playback-note" role="status">{t("Playback stopped after a device failure.")}</p>}
        {linkedSession?.status === "degraded" && <p className="connection-playback-note" role="status">{t("Continuing without {devices}. Stop and start again to rejoin.", { devices: linkedSession.retired_devices.map((device) => device === "handy" ? t("The Handy") : t("Coyote")).join(", ") })}</p>}
        {/* Each device section stays mounted while hidden: the Bluetooth bridge
            and the Intiface and Coyote panels own live sessions. */}
        <div className="connection-section connection-handy" hidden={!usesHandy}>
          <div className="connection-device-head">
            <div className="connection-device-title">
              <h3 id={handyTitleId}>{t("The Handy")}</h3>
              {canChooseDevice
                ? <ConnectionMethodPicker titleId={handyTitleId} value={owner} fallbackLabel={methodLabel} methods={methods} busy={selectionBusy} onChange={(next) => void changeMethod(next)} />
                : <p className="connection-method-static">{methodLabel}</p>}
            </div>
            {<button type="button" className="icon-button connection-remove"
              aria-label={t("Remove {device}", { device: t("The Handy") })} title={t("Removing a connection stops motion.")}
              disabled={!canChooseDevice} aria-busy={selectionBusy || undefined}
              onClick={() => { focusAfterRemoval.current = true; void changeConnection("handy", "remove"); }}><CloseIcon size={16} /></button>}
          </div>
          <label className="connection-participation toggle-line"><span className="toggle"><input type="checkbox" aria-label={t("Use {device} in playback", { device: t("The Handy") })} checked={handyIncluded} disabled={!canChooseDevice || selectionBusy} onChange={(event) => void changeConnection("handy", event.target.checked ? "include" : "exclude")} /><span className="track" aria-hidden="true" /></span><span>{t("Use in playback")}</span></label>
          {usesHandy && <ConnectionArtwork phase={handyPhase} />}
          {usesHandy && <div className="connection-current" aria-live="polite">
            <span className="status-dot" data-state={handyPhase === "connected" ? "ok" : handyPhase === "connecting" || handyPhase === "initializing" ? "pending" : handyPhase === "error" ? "error" : "idle"} aria-hidden="true" />
            <span className="connection-current-copy">
              <strong>{handyName}</strong>
              <small>{translateKnown(handyStatus)}</small>
            </span>
            {owner === "cloud_rest" && (
              <button
                type="button"
                className="icon-button connection-current-check"
                aria-label={t("Check Cloud connection")}
                title={t("Check connection")}
                disabled={locked || cloudBusy || !keySet}
                onClick={() => void probeCloud("check")}
              >
                <RefreshIcon size={17} />
              </button>
            )}
          </div>}

          <div className="connection-provider-actions">
            {usesHandy && owner === "cloud_rest" && (
              <>
                <form className="connection-key-form" onSubmit={(event) => void saveCloudKey(event)}>
                  <div className="connection-key-label">
                    <label htmlFor="connection-manager-key">{t("Handy connection key")}</label>
                    <span>{keySet ? t("Saved") : t("Required")}</span>
                  </div>
                  <div className="connection-key-entry">
                    <input
                      id="connection-manager-key"
                      type="password"
                      autoComplete="off"
                      spellCheck={false}
                      placeholder={keySet ? t("Leave blank to keep saved key") : t("Paste connection key")}
                      value={cloudKey}
                      disabled={locked || cloudKeyBusy}
                      onChange={(event) => setCloudKey(event.target.value)}
                    />
                    <button type="submit" className="btn btn-secondary" disabled={locked || cloudKeyBusy || !cloudKey.trim()}>
                      {cloudKeyBusy ? t("Saving") : t("Save key")}
                    </button>
                  </div>
                </form>
                <button
                  type="button"
                  className={`btn ${connected ? "btn-secondary" : "btn-primary"}`}
                  disabled={locked || cloudBusy || !keySet}
                  onClick={() => void (connected ? disconnectCloud() : probeCloud("connect"))}
                >
                  <WirelessIcon />
                  {cloudAction === "disconnect" ? t("Disconnecting") : cloudAction === "connect" ? t("Connecting") : connected ? t("Disconnect") : t("Connect")}
                </button>
                <p className="connection-provider-meta">{bundledApplicationID ? t("Built-in Handy API v3 ID") : t("Developer API v3 ID override")}</p>
              </>
            )}
            <BluetoothBridge
              visible={usesHandy && owner === "browser_bluetooth"}
              canConfigureHost={canConfigureHost}
              locked={locked}
              backendOnline={backendOnline}
              initial={state?.bluetooth_bridge as BluetoothBridgeSnapshot | undefined}
              onStateChange={onBluetoothState}
            />
            <IntifacePanel
              visible={usesHandy && owner === "intiface"}
              locked={locked}
              dirty={false}
              initial={state?.intiface_transport}
              onActivityChange={onIntifaceActivity}
              onConnectionAttemptError={onIntifaceAttemptError}
              onSnapshotChange={onIntifaceSnapshot}
            />
          </div>
        </div>

        <div className="connection-section connection-coyote" hidden={!usesCoyote}>
          <div className="connection-device-head">
            <div className="connection-device-title">
              <h3 id={coyoteTitleId}>{t("Coyote")}</h3>
              <p className="connection-method-static">{t("Coyote Bluetooth")}</p>
            </div>
            {<button type="button" className="icon-button connection-remove"
              aria-label={t("Remove {device}", { device: t("Coyote") })} title={t("Removing a connection stops motion.")}
              disabled={!canChooseDevice} aria-busy={selectionBusy || undefined}
              onClick={() => { focusAfterRemoval.current = true; void changeConnection("coyote", "remove"); }}><CloseIcon size={16} /></button>}
          </div>
          <label className="connection-participation toggle-line"><span className="toggle"><input type="checkbox" aria-label={t("Use {device} in playback", { device: t("Coyote") })} checked={coyoteIncluded} disabled={!canChooseDevice || selectionBusy} onChange={(event) => void changeConnection("coyote", event.target.checked ? "include" : "exclude")} /><span className="track" aria-hidden="true" /></span><span>{t("Use in playback")}</span></label>
          <CoyotePanel
            playbackActive={Boolean(state?.motion?.engine?.running || state?.motion?.engine?.starting || state?.motion?.engine?.completing)}
            visible={usesCoyote}
            locked={locked}
            canConfigureHost={canConfigureHost}
            safetyAcknowledged={coyoteSafetyAcknowledged}
            initial={coyote}
            stopSequence={state?.stop_sequence}
            onChanged={refresh}
          />
          {usesCoyote && coyoteSafetyAcknowledged && !coyote?.status.connected && (
            <p className="hint-block connection-coyote-route">{t("The Coyote uses the Bluetooth of the computer running MagicHandy, not this browser.")}</p>
          )}
        </div>

        <div className="connection-section connection-limits">
          <div className="connection-section-head">
            <h3>{t("Motion")}</h3>
            <span>{t("Applies immediately")}</span>
          </div>
          <QuickSettings section="connection" />
        </div>
      </section>

      <button
        ref={triggerRef}
        type="button"
        className="connection-manager-trigger"
        aria-label={t("{device} {status}; {action} connection manager", { device: deviceName, status: statusText, action: open ? t("close") : t("open") })}
        aria-controls="connection-manager-panel"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
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

function normalizeOutput(value: string | undefined): DeviceOutput {
  return value === "none" || value === "coyote" || value === "handy_coyote" ? value : "handy";
}

function connectionProviderName(owner: string, initializing: boolean) {
  if (initializing) return "Loading provider";
  if (owner === "browser_bluetooth") return "Browser Bluetooth";
  if (owner === "intiface") return "Intiface Central";
  if (owner === "cloud_rest") return "Cloud REST";
  return "Not configured";
}

function ConnectionArtwork({ phase }: { phase: ConnectionPhase }) {
  return (
    <svg
      className="connection-artwork"
      data-phase={phase}
      viewBox="0 0 360 120"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={phase === "initializing" ? t("The Handy connection status loading") : phase === "connecting" ? t("The Handy wireless connection in progress") : t("The Handy wireless connection")}
    >
      <g className="connection-signal" aria-hidden="true">
        {SIGNAL_PATHS.map((path, index) => <path key={path} d={path} style={{ "--signal-index": index } as CSSProperties} />)}
      </g>
      <g className="connection-error-mark" data-visible={phase === "error"} aria-hidden="true">
        <path d="m167 11 19 19" />
        <path d="m186 11-19 19" />
      </g>
      <g className="connection-handy" aria-hidden="true">
        <rect className="connection-handy-body" x="146" y="42" width="27" height="70" rx="13.5" />
        <path className="connection-handy-body" d="M180 112v-22.5c0-7.5 6-13.5 13.5-13.5s13.5 6 13.5 13.5V112Z" />
        <circle className="connection-handy-led" cx="159.5" cy="77" r="3" />
        <rect
          className="connection-handy-marker"
          data-state={phase === "connected" ? "connected" : phase === "connecting" ? "connecting" : phase === "initializing" ? "initializing" : "disconnected"}
          x="216"
          y="105"
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
  if (input.phase === "connecting") return input.owner === "cloud_rest" ? "Checking connection" : input.owner === "intiface" ? "Finding a linear device" : "Connecting";
  if (input.phase === "error" && input.owner === "cloud_rest") return "Cloud connection failed";
  if (input.phase === "error" && input.owner === "intiface") return "Intiface connection failed";
  if (input.owner === "cloud_rest") {
    if (!input.keySet) return "Connection key required";
    return input.cloudVerified ? "Cloud connection ready" : "Cloud disconnected";
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
