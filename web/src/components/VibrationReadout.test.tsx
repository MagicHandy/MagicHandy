import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { IntifaceTransportSnapshot, MotionInfo, PublicSettings } from "../api/types";
import { showsVibration, VibrationReadout } from "./VibrationReadout";

describe("vibration readout", () => {
  it("shows each actual commanded motor step, including silence", () => {
    const vibrator = { motors: [{ index: 0, level_percent: 20 }, { index: 1, level_percent: 0 }] } as IntifaceTransportSnapshot["vibrator"];
    render(<VibrationReadout vibrator={vibrator} />);
    expect(screen.getByRole("meter", { name: "Motor 1: 20%" })).toHaveAttribute("value", "20");
    expect(screen.getByRole("meter", { name: "Motor 2: 0%" })).toHaveAttribute("value", "0");
    expect(screen.getByText("Last commands sent to Intiface, not measured vibration.")).toBeInTheDocument();
  });

  it("does not present an idle Handy as moving during a vibration-only run", () => {
    const settings = { device: { devices: ["handy", "vibrator"], playback_devices: ["handy", "vibrator"] } } as unknown as PublicSettings;
    const motion = { engine: { target: { vibration_only: true } } } as unknown as MotionInfo;
    expect(showsVibration(settings, motion)).toBe(true);
    expect(showsVibration(settings, null)).toBe(false);
  });

  it("reports a missing connection without inventing a motor value", () => {
    render(<VibrationReadout />);
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  });
});
