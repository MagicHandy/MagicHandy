import { additionalHandyDevice } from "./additional-handy";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CoyoteStatus, CoyoteTransportSnapshot, IntifaceTransportSnapshot, PublicSettings } from "../api/types";
import { coyoteDevice } from "./coyote";
import { handyDevice } from "./handy";
import { EMPTY_INTIFACE } from "./runtime";
import { chipSummary } from "./status";
import type { DeviceContext } from "./types";
import { vibratorDevice } from "./vibrator";

vi.mock("../api/client", () => ({
  api: {
    cloudConnect: vi.fn(),
    cloudDisconnect: vi.fn(),
    coyoteConnect: vi.fn(),
    coyoteDisconnect: vi.fn(),
    coyoteStrength: vi.fn(),
    coyoteTest: vi.fn(),
    coyoteZero: vi.fn(),
    intifaceConnect: vi.fn(),
    intifaceDisconnect: vi.fn(),
    nativeBluetoothConnect: vi.fn(),
    nativeBluetoothDisconnect: vi.fn(),
    vibratorIntensity: vi.fn(),
    vibratorTest: vi.fn(),
  },
}));

const coyoteStatus = (patch: Partial<CoyoteStatus> = {}): CoyoteStatus => ({
  connected: true, connecting: false, simulated: false, battery_percent: 82,
  strength_a: 12, strength_b: 8, strength_known: true, strength_source: "reported", strength_pending: false,
  limits_applied: true, ceiling_a: 30, ceiling_b: 30, playback_state: "idle", reference_pulse: false,
  commanded_a: 0, commanded_b: 0, packets_sent: 0, write_failures: 0, starved_packets: 0, max_send_lateness_ms: 0,
  ...patch,
});

function intiface(patch: Omit<Partial<IntifaceTransportSnapshot>, "status"> & { status?: Partial<IntifaceTransportSnapshot["status"]> } = {}): IntifaceTransportSnapshot {
  return { ...EMPTY_INTIFACE, ...patch, status: { ...EMPTY_INTIFACE.status, ...patch.status } };
}

function makeCtx(patch: Partial<DeviceContext> = {}, settings: Partial<PublicSettings> = {}): DeviceContext {
  const ctx: DeviceContext = {
    state: null,
    settings: {
      device: { hsp_dispatch_owner: "cloud_rest", intiface_server_address: "ws://127.0.0.1:12345", firmware_api_requirement: "firmware_v4_api_v3_required", api_application_id_source: "bundled_app_id", connection_key_set: true, configured_devices: ["handy", "coyote", "vibrator"], playback_devices: ["handy", "coyote", "vibrator"] },
      estim: { safety_acknowledged: true, max_strength_a: 30, max_strength_b: 30, follow: "stroke", frequency_min_hz: 40, frequency_max_hz: 100, balance: 0, swap_channels: false, timing_offset_ms: 0, frequency_balance_a: 160, frequency_balance_b: 160, intensity_balance_a: 0, intensity_balance_b: 0 },
      motion: { speed_min_percent: 20, speed_max_percent: 80, stroke_min_percent: 0, stroke_max_percent: 100, reverse_direction: false, apply_video_speed_limit: false, style: "balanced", handy_model: "handy_original" },
      ...settings,
    } as PublicSettings,
    backendOnline: true,
    readOnly: false,
    locked: false,
    canConfigureHost: true,
    configured: ["handy", "coyote", "vibrator"],
    playing: ["handy", "coyote", "vibrator"],
    engineActive: false,
    retired: [],
    stopSequence: 7,
    intiface: EMPTY_INTIFACE,
    coyote: undefined,
    native: undefined,
    handys: [],
    setHandy: vi.fn(),
    bluetooth: { connected: false, connecting: false, status: "disconnected", deviceName: "", supported: true },
    busy: {},
    failed: {},
    run: vi.fn(async (_device, _action, request) => { await request(); return true; }),
    send: vi.fn((request: () => Promise<unknown>) => { void request(); }),
    setFailed: vi.fn(),
    setCoyote: vi.fn(),
    setIntiface: vi.fn(),
    browserBluetooth: { connect: vi.fn(), disconnect: vi.fn() },
    ...patch,
  };
  return ctx;
}

const coyoteSnapshot = (status: Partial<CoyoteStatus> = {}, available = true): CoyoteTransportSnapshot => ({ output: "handy_coyote", available, status: coyoteStatus(status) });

beforeEach(() => vi.clearAllMocks());

describe("The Handy's descriptor", () => {
  it("asks for the connection key before Cloud can connect", () => {
    const model = handyDevice.model(makeCtx({}, { device: { ...makeCtx().settings!.device, connection_key_set: false } }));
    expect(model.status).toBe("needs_setup");
    expect(model.details).toEqual(["connection key"]);
    expect(model.action).toEqual({ kind: "setup" });
  });

  it("reads Playing only while connected and part of the current run", () => {
    const connected = makeCtx({ state: { cloud_transport: { connected: true, last_latency_ms: 64 } } });
    expect(handyDevice.model(connected)).toMatchObject({ status: "connected", details: ["Cloud", "64 ms"], action: { kind: "disconnect" } });
    expect(handyDevice.model({ ...connected, engineActive: true }).status).toBe("playing");
    expect(handyDevice.model({ ...connected, engineActive: true, playing: ["coyote"] }).status).toBe("connected");
    expect(handyDevice.model({ ...connected, engineActive: true, retired: ["handy"] })).toMatchObject({ status: "lost", details: ["left this run"], action: { kind: "reconnect" } });
  });

  it("counts an Intiface session as the Handy's only while it holds its own lease", () => {
    const ctx = (snapshot: IntifaceTransportSnapshot) => makeCtx({ intiface: snapshot }, { device: { ...makeCtx().settings!.device, hsp_dispatch_owner: "intiface" } });
    const vibratorOnly = intiface({ leases: ["vibrator"], status: { connected: true, selected_device_index: 2, selected_actuator_index: 0 } });
    expect(handyDevice.model(ctx(vibratorOnly)).status).toBe("not_connected");
    const chooseDevice = intiface({ leases: ["handy"], status: { connected: true, scanning: true } });
    expect(handyDevice.model(ctx(chooseDevice))).toMatchObject({ status: "needs_setup", details: ["choose a device"] });
    // A scan keeps running once an actuator is chosen; the Handy is connected.
    const chosen = intiface({ leases: ["handy"], status: { connected: true, scanning: true, selected_device_index: 2, selected_actuator_index: 0 } });
    expect(handyDevice.model(ctx(chosen)).status).toBe("connected");
  });

  it("connects only on an explicit action and keeps host Bluetooth to the host", async () => {
    const owner = { device: { ...makeCtx().settings!.device, hsp_dispatch_owner: "native_bluetooth" } };
    const native = { available: true, supported: true, remembered_device: false, status: { connected: false, connecting: false, simulated: false, status: "idle", message: "", clock_synced: false, write_bytes: 0, playback_state: "idle" } };
    const model = handyDevice.model(makeCtx({ native }, owner));
    expect(api.nativeBluetoothConnect).not.toHaveBeenCalled();
    model.action?.run?.();
    await vi.waitFor(() => expect(api.nativeBluetoothConnect).toHaveBeenCalledOnce());

    const guest = handyDevice.model(makeCtx({ native, canConfigureHost: false }, owner));
    expect(guest.action).toMatchObject({ kind: "connect", disabled: true });
    const missing = handyDevice.model(makeCtx({ native: { ...native, available: false } }, owner));
    expect(missing).toMatchObject({ status: "needs_setup", details: ["Bluetooth helper missing"] });
  });

  it("reconnects a browser Bluetooth link that failed", () => {
    const owner = { device: { ...makeCtx().settings!.device, hsp_dispatch_owner: "browser_bluetooth" } };
    const failed = handyDevice.model(makeCtx({ bluetooth: { connected: false, connecting: false, status: "error", deviceName: "", supported: true } }, owner));
    expect(failed).toMatchObject({ status: "lost", action: { kind: "reconnect" } });
  });

  it("summarizes the method and calibration for Settings > Devices", () => {
    expect(handyDevice.model(makeCtx()).summary).toBe("Handy Cloud · Original · 110 mm");
  });
});

describe("The Coyote's descriptor", () => {
  it("needs the safety notes before anything else", () => {
    const ctx = makeCtx({ coyote: coyoteSnapshot({ connected: false }) }, { estim: { ...makeCtx().settings!.estim!, safety_acknowledged: false } });
    expect(coyoteDevice.model(ctx)).toMatchObject({ status: "needs_setup", details: ["safety notes first"], action: { kind: "setup" } });
  });

  it("shows the device's own strength of its ceiling and steps by one", async () => {
    const ctx = makeCtx({ coyote: coyoteSnapshot() });
    const model = coyoteDevice.model(ctx);
    expect(model.details).toEqual(["Battery 82%"]);
    const [a, b] = model.levels!.levels;
    expect(a).toMatchObject({ label: "Channel A", value: 12, ceiling: 30, canLower: true, canRaise: true, canTest: true });
    expect(b.value).toBe(8);
    a.raise();
    a.lower();
    await vi.waitFor(() => expect(api.coyoteStrength).toHaveBeenCalledTimes(2));
    expect(api.coyoteStrength).toHaveBeenNthCalledWith(1, "a", 1, 7);
    expect(api.coyoteStrength).toHaveBeenNthCalledWith(2, "a", -1, 7);
    expect(model.readout).toBe("A 12 · B 8");
  });

  it("blocks raising and Test until the strength is confirmed, but always allows lowering", () => {
    const pending = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ strength_known: false, strength_a: 0 }) })).levels!.levels[0];
    expect(pending).toMatchObject({ value: null, canLower: true, canRaise: false, canTest: false });
    const atCeiling = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ strength_a: 30 }) })).levels!.levels[0];
    expect(atCeiling).toMatchObject({ canRaise: false, canLower: true });
  });

  it("keeps raising and Test for the host and lowering for everyone", () => {
    const guest = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot(), canConfigureHost: false })).levels!.levels[0];
    expect(guest).toMatchObject({ canLower: true, canRaise: false, canTest: false });
  });

  it("plays a test only while motion is paused, with the current Stop sequence", async () => {
    const running = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot(), engineActive: true })).levels!.levels[0];
    expect(running.canTest).toBe(false);
    const level = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot() })).levels!.levels[1];
    level.test();
    await vi.waitFor(() => expect(api.coyoteTest).toHaveBeenCalledWith("b", 7));
    const zero = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ strength_b: 0 }) })).levels!.levels[1];
    expect(zero.canTest).toBe(false);
  });

  it("names a delivered zero command apart from a device report", () => {
    const model = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ strength_source: "zero_command", strength_a: 0, strength_b: 0 }) }));
    expect(model.levels?.note).toMatch(/zero command accepted/i);
    expect(coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot() })).levels?.note).toBeUndefined();
  });

  it("reports a dropped Coyote as lost and reconnects it", () => {
    const model = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ connected: false, playback_state: "stale" }) }));
    expect(model).toMatchObject({ status: "lost", details: ["dropped"], action: { kind: "reconnect" } });
    expect(model.levels).toBeUndefined();
  });

  it("asks for setup when the Bluetooth helper is missing", () => {
    const model = coyoteDevice.model(makeCtx({ coyote: coyoteSnapshot({ connected: false }, false) }));
    expect(model).toMatchObject({ status: "needs_setup", details: ["Bluetooth helper missing"] });
  });
});

describe("The vibrator's descriptor", () => {
  const vibrator = (patch: Partial<NonNullable<IntifaceTransportSnapshot["vibrator"]>> = {}) => ({
    selected: true, device_index: 3, device_name: "Simulated Vibrator", motors: [{ index: 0, step_count: 20 }],
    intensity_percent: 15, ceiling_percent: 40, effective_ceiling_percent: 40, level_percent: 0, playback_state: "idle", testing: false,
    commanded_count: 0, acknowledged_count: 0, rejected_count: 0, timeout_count: 0, ...patch,
  });

  it("does not count a session that only the Handy holds", () => {
    const model = vibratorDevice.model(makeCtx({ intiface: intiface({ leases: ["handy"], status: { connected: true } }) }));
    expect(model).toMatchObject({ status: "not_connected", action: { kind: "connect" } });
  });

  it("asks for a device once its lease is held", () => {
    const model = vibratorDevice.model(makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true } }) }));
    expect(model).toMatchObject({ status: "needs_setup", details: ["choose a device"], action: { kind: "setup" } });
  });

  it("raises with the Stop sequence, lowers without it, and stops at the ceiling", async () => {
    const ctx = makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator() }) });
    const [level] = vibratorDevice.model(ctx).levels!.levels;
    expect(level).toMatchObject({ label: "Intensity", value: 15, ceiling: 40, unit: "%" });
    level.raise();
    level.lower();
    await vi.waitFor(() => expect(api.vibratorIntensity).toHaveBeenCalledTimes(2));
    expect(api.vibratorIntensity).toHaveBeenNthCalledWith(1, 20, 7);
    expect(api.vibratorIntensity).toHaveBeenNthCalledWith(2, 10, undefined);
    const top = vibratorDevice.model(makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator({ intensity_percent: 40 }) }) })).levels!.levels[0];
    expect(top.canRaise).toBe(false);
  });

  it("tests only while motion is paused and never at zero", () => {
    const running = makeCtx({ engineActive: true, intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator() }) });
    expect(vibratorDevice.model(running).levels!.levels[0].canTest).toBe(false);
    const zero = makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator({ intensity_percent: 0 }) }) });
    expect(vibratorDevice.model(zero).levels!.levels[0]).toMatchObject({ canTest: false, canLower: false });
  });

  it("can clear the backend's post-Stop resume permission without raising output", async () => {
    const ctx = makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator({ intensity_percent: 0, resume_intensity_percent: 35, silent_reason: "emergency_stop" }) }) });
    const levels = vibratorDevice.model(ctx).levels!;
    expect(levels.note).toContain("35% intensity");
    expect(levels.zero).toMatchObject({ label: "Keep vibration off", disabled: false });
    levels.zero!.run();
    await vi.waitFor(() => expect(api.vibratorIntensity).toHaveBeenCalledWith(0, undefined));
    expect(vibratorDevice.model({ ...ctx, locked: true }).levels!.zero!.disabled).toBe(true);
    const cleared = { ...ctx, intiface: { ...ctx.intiface, vibrator: vibrator({ intensity_percent: 0 }) } };
    expect(vibratorDevice.model(cleared).levels!.zero).toBeUndefined();
  });

  it("says when the motors' steps lower the effective ceiling", () => {
    const model = vibratorDevice.model(makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator({ effective_ceiling_percent: 35 }) }) }));
    expect(model.levels?.note).toMatch(/highest level is 35%/);
  });

  it("sends a device that dropped out of a live session back to its page", () => {
    const model = vibratorDevice.model(makeCtx({ intiface: intiface({ leases: ["vibrator"], status: { connected: true }, vibrator: vibrator({ playback_state: "stale" }) }) }));
    expect(model).toMatchObject({ status: "lost", action: { kind: "setup" } });
  });
});

describe("the device chip", () => {
  const ctx = makeCtx({ state: { cloud_transport: { connected: true } }, coyote: coyoteSnapshot({ connected: false, playback_state: "stale" }) });

  it("shows one device's own status", () => {
    expect(chipSummary([handyDevice.model(ctx)])).toEqual({ title: "The Handy", status: "Connected · Cloud" });
  });

  it("names what plays, then the one problem", () => {
    const models = [handyDevice.model(ctx), coyoteDevice.model(ctx), vibratorDevice.model(ctx)];
    expect(chipSummary(models)).toEqual({ title: "The Handy + Coyote + Vibrator", status: "Coyote connection lost" });
  });

  it("reads Chat only without devices", () => {
    expect(chipSummary([])).toEqual({ title: "Devices", status: "Chat only" });
  });
});


describe("named Handy state", () => {
  const id = "handy-010203040506" as const;
  it("uses the backend's participation, connection and retired state", () => {
    const ctx = makeCtx({ configured: ["handy", id], playing: [id], handys: [{ id, connected: true, connecting: false, status: "connected", simulated: false, native_available: true, native_supported: true }] });
    ctx.settings!.device.handy_connections = [{ id, name: "Guest Handy", method: "cloud_rest", handy_model: "handy_original", connection_key_set: true, remembered_device: false }];
    const descriptor = additionalHandyDevice(id, ctx.settings);
    expect(descriptor.model(ctx)).toMatchObject({ name: "Guest Handy", status: "connected", inPlayback: true });
    expect(descriptor.model({ ...ctx, engineActive: true }).status).toBe("playing");
    expect(descriptor.model({ ...ctx, engineActive: true, retired: [id] }).status).toBe("lost");
    expect(descriptor.model({ ...ctx, playing: [], engineActive: true })).toMatchObject({ status: "connected", inPlayback: false });
  });
});
