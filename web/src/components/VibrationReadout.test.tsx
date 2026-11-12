import { describe, expect, it } from "vitest";
import type { MotionInfo, PublicSettings } from "../api/types";
import { showsVibration } from "./VibrationReadout";

describe("vibration readout", () => {
  it("does not present an idle Handy as moving during a vibration-only run", () => {
    const settings = { device: { devices: ["handy", "vibrator"], playback_devices: ["handy", "vibrator"] } } as unknown as PublicSettings;
    const motion = { engine: { running: true, target: { vibration_only: true } } } as unknown as MotionInfo;
    expect(showsVibration(settings, motion)).toBe(true);
    expect(showsVibration(settings, null)).toBe(false);
  });

  it("recognizes position-capable Ultra and ignores a stopped vibration-only target", () => {
    const settings = { device: { playback_devices: ["autoblow", "vibrator"] } } as unknown as PublicSettings;
    expect(showsVibration(settings, null)).toBe(false);
    const motion = { engine: { running: false, target: { vibration_only: true } } } as unknown as MotionInfo;
    expect(showsVibration(settings, motion)).toBe(false);
  });
});
