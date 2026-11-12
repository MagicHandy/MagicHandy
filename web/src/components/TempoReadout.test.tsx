import { describe, expect, it } from "vitest";
import type { MotionInfo, PublicSettings } from "../api/types";
import { routineName, showsTempo, tempoLeads, tempoModeLabel } from "./TempoReadout";

function playing(devices: string[]): PublicSettings {
  return { device: { configured_devices: devices, playback_devices: devices } } as unknown as PublicSettings;
}

describe("tempo readout", () => {
  it("leads only while no position device plays", () => {
    expect(tempoLeads(playing(["vacuglide"]))).toBe(true);
    expect(tempoLeads(playing(["vacuglide", "vibrator"]))).toBe(true);
    expect(tempoLeads(playing(["handy", "vacuglide"]))).toBe(false);
    expect(tempoLeads(playing(["autoblow", "vacuglide"]))).toBe(false);
    expect(tempoLeads(playing(["handy"]))).toBe(false);
  });

  it("shows a tempo-only run even beside an idle Handy", () => {
    const motion = { engine: { running: true, target: { tempo_only: true } } } as unknown as MotionInfo;
    expect(showsTempo(playing(["handy", "vacuglide"]), motion)).toBe(true);
    expect(showsTempo(playing(["handy", "vacuglide"]), null)).toBe(false);
  });

  it("does not keep the old tempo controls after Stop and a device selection change", () => {
    const motion = { engine: { running: false, target: { tempo_only: true } } } as unknown as MotionInfo;
    expect(showsTempo(playing(["handy"]), motion)).toBe(false);
  });

  it("names a routine and never invents a mode while stopped", () => {
    expect(tempoModeLabel({ mode: "routine", routine: "bursts", level_percent: 50 }, true)).toBe(routineName("bursts"));
    expect(tempoModeLabel({ mode: "ramp", level_percent: 50 }, true)).toBe("Gliding to a new tempo");
    expect(tempoModeLabel({ mode: "steady", level_percent: 70 }, false)).toBe("Stopped");
  });
});
