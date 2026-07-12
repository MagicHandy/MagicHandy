import { useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { IntifaceTransportSnapshot } from "../api/types";
import { useToast } from "../state/app-state";

interface IntifacePanelProps {
  visible: boolean;
  locked: boolean;
  dirty: boolean;
  initial?: IntifaceTransportSnapshot;
}

const emptySnapshot: IntifaceTransportSnapshot = {
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

export function IntifacePanel({ visible, locked, dirty, initial }: IntifacePanelProps) {
  const { show } = useToast();
  const [snapshot, setSnapshot] = useState(initial ?? emptySnapshot);
  const [busy, setBusy] = useState(false);
  const [choice, setChoice] = useState("");

  useEffect(() => {
    if (initial) setSnapshot(initial);
  }, [initial]);

  useEffect(() => {
    if (!visible) return;
    void api.intifaceStatus().then(setSnapshot).catch(() => undefined);
  }, [visible]);

  const choices = useMemo(() => (snapshot.status.devices ?? []).flatMap((device) =>
    device.linear_actuators.map((actuator) => ({
      value: `${device.device_index}:${actuator.index}`,
      label: `${device.device_name} - ${actuator.feature_descriptor || actuator.actuator_type || `Linear ${actuator.index + 1}`}`,
    }))), [snapshot.status.devices]);

  useEffect(() => {
    const device = snapshot.status.selected_device_index;
    const actuator = snapshot.status.selected_actuator_index;
    if (device !== undefined && actuator !== undefined) setChoice(`${device}:${actuator}`);
    else if (choices.length === 1) setChoice(choices[0].value);
  }, [choices, snapshot.status.selected_actuator_index, snapshot.status.selected_device_index]);

  if (!visible) return null;

  async function run(action: () => Promise<IntifaceTransportSnapshot>, success: string) {
    setBusy(true);
    try {
      const next = await action();
      setSnapshot(next);
      show(success);
    } catch (error) {
      show(error instanceof Error ? error.message : "Intiface request failed.", "error");
    } finally {
      setBusy(false);
    }
  }

  function selectActuator() {
    const [deviceIndex, actuatorIndex] = choice.split(":").map(Number);
    if (!Number.isInteger(deviceIndex) || !Number.isInteger(actuatorIndex)) return;
    void run(() => api.intifaceSelect(deviceIndex, actuatorIndex), "Intiface actuator selected.");
  }

  const statusText = snapshot.status.connected
    ? snapshot.status.selected_device_index === undefined ? "Connected - select a linear actuator" : "Connected - ready"
    : "Disconnected";

  return (
    <div className="transport-panel" aria-live="polite">
      <div className="transport-summary">
        <span className="status-dot" data-state={snapshot.status.connected ? "connected" : "disconnected"} aria-hidden="true" />
        <span>{statusText}</span>
      </div>
      {dirty && <p className="form-status">Save the dispatch owner and server address before connecting.</p>}
      <div className="row-actions">
        {snapshot.status.connected ? (
          <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => void run(api.intifaceDisconnect, "Intiface disconnected.")}>Disconnect</button>
        ) : (
          <button type="button" className="btn btn-secondary" disabled={locked || dirty || busy} onClick={() => void run(api.intifaceConnect, "Intiface connected.")}>Connect</button>
        )}
        {snapshot.status.connected && (
          <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => void run(snapshot.status.scanning ? api.intifaceStopScan : api.intifaceStartScan, snapshot.status.scanning ? "Intiface scan stopped." : "Intiface scan started.")}>{snapshot.status.scanning ? "Stop scan" : "Scan devices"}</button>
        )}
      </div>
      {snapshot.status.connected && choices.length > 0 && (
        <div className="transport-selection">
          <label className="field">
            <span className="label">Linear actuator</span>
            <select value={choice} disabled={locked || busy} onChange={(event) => setChoice(event.target.value)}>
              <option value="">Select an actuator</option>
              {choices.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
          </label>
          <button type="button" className="btn btn-secondary" disabled={locked || busy || !choice} onClick={selectActuator}>Use actuator</button>
        </div>
      )}
      {snapshot.status.connected && choices.length === 0 && !snapshot.status.scanning && (
        <p className="form-status">No linear actuators discovered. Start a scan after the device is available to Intiface Central.</p>
      )}
    </div>
  );
}
