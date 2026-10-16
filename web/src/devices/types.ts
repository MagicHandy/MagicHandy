// The device kit's data model. Each device fills in a descriptor; the panel
// row, the device page, the top-bar chip and the Add device list all render
// from it (docs/ui-design-guidelines.md, Device kit).
import type { ComponentType } from "react";
import type {
  AppState,
  CoyoteTransportSnapshot,
  DeviceId,
  DeviceOptionsPatch,
  IntifaceTransportSnapshot,
  PublicSettings,
} from "../api/types";
import type { NativeBluetoothSnapshot } from "../api/native-bluetooth-types";
import type { BluetoothBridgeState } from "../components/BluetoothBridge";
import type { DeviceGlyph } from "./glyphs";

/** One word per state, with its dot (Device kit, Status words). */
export type DeviceStatusWord = "playing" | "connected" | "connecting" | "not_connected" | "needs_setup" | "lost";

/** The one action a device row offers for its state. */
export interface DeviceAction {
  kind: "connect" | "disconnect" | "setup" | "reconnect";
  /** Absent for Set up, which opens the device page. */
  run?: () => void;
  disabled?: boolean;
  busy?: boolean;
  /** Why the action cannot run, shown as its tooltip and description. */
  reason?: string;
}

/** A live level: Coyote strength A and B, the vibrator's intensity. */
export interface DeviceLevelModel {
  id: string;
  label: string;
  /** Null while the device has not confirmed it. */
  value: number | null;
  ceiling: number;
  unit: "" | "%";
  canLower: boolean;
  canRaise: boolean;
  canTest: boolean;
  lower: () => void;
  raise: () => void;
  test: () => void;
  lowerLabel: string;
  raiseLabel: string;
  testLabel: string;
}

export interface DeviceLevelGroup {
  /** The visible caption above several levels. */
  label: string;
  /** The group's accessible name, which says whose levels these are. */
  name: string;
  levels: DeviceLevelModel[];
  /** Coyote's Zero both. */
  zero?: { label: string; run: () => void; disabled: boolean };
  /** A note under the levels, such as an unconfirmed zero. */
  note?: string;
}

export interface DeviceModel {
  id: DeviceId;
  name: string;
  status: DeviceStatusWord;
  /** What follows the status word: method, latency, battery. */
  details: string[];
  action: DeviceAction | null;
  inPlayback: boolean;
  levels?: DeviceLevelGroup;
  /** The levels as text, for a tab that only watches. */
  readout?: string;
  /** The Settings > Devices row: how it connects and how it is set. */
  summary: string;
}

/** Live device state for one render: backend snapshots plus this tab's
 * in-flight requests. Nothing here is a parallel model of device state. */
export interface DeviceContext {
  state: AppState | null;
  settings?: PublicSettings;
  backendOnline: boolean;
  readOnly: boolean;
  /** Offline or read-only: device commands are unavailable. */
  locked: boolean;
  canConfigureHost: boolean;
  configured: DeviceId[];
  playing: DeviceId[];
  /** Motion is running, starting or completing. */
  engineActive: boolean;
  /** Devices a degraded linked run left behind. */
  retired: DeviceId[];
  stopSequence?: number;
  intiface: IntifaceTransportSnapshot;
  coyote?: CoyoteTransportSnapshot;
  native?: NativeBluetoothSnapshot;
  bluetooth: BluetoothBridgeState;
  busy: Partial<Record<DeviceId, string>>;
  failed: Partial<Record<DeviceId, boolean>>;
  /** Runs a device request with busy state, a toast on failure and a
   * refresh. A request that resolves to text shows it as the success toast. */
  run: (device: DeviceId, action: string, request: () => Promise<string | void>) => Promise<boolean>;
  /** Sends a live step (strength, intensity) without busy state or refresh. */
  send: (request: () => Promise<unknown>) => void;
  setFailed: (device: DeviceId, failed: boolean) => void;
  setCoyote: (snapshot: CoyoteTransportSnapshot) => void;
  setIntiface: (snapshot: IntifaceTransportSnapshot) => void;
  browserBluetooth: { connect: () => void; disconnect: () => void };
}

export interface DevicePageSection {
  id: string;
  title: () => string;
  Body: ComponentType<DeviceSectionProps>;
}

export interface DeviceSectionProps {
  ctx: DeviceContext;
  model: DeviceModel;
}

export interface DeviceDescriptor {
  id: DeviceId;
  name: () => string;
  Glyph: DeviceGlyph;
  /** The Add device list: what it is and how it connects. */
  addDetail: () => string;
  model: (ctx: DeviceContext) => DeviceModel;
  /** The device page, top to bottom; Remove follows the last section. */
  sections: DevicePageSection[];
  removeNote: () => string;
  /** A timing offset, under Settings > Devices > Playback. */
  timing?: { value: (settings: PublicSettings) => number; patch: (ms: number) => DeviceOptionsPatch };
  /** The shared connection this device reaches its hardware through. */
  connection: (settings: PublicSettings | undefined) => "intiface" | "host_bluetooth" | null;
}
