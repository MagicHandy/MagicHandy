import { describe, expect, it } from "vitest";
import type { EngineSnapshot } from "../api/types";
import { ownsActiveMotion } from "./motion";

function engine(overrides: Partial<EngineSnapshot>): EngineSnapshot {
  return {
    running: false,
    paused: false,
    ...overrides,
  };
}

describe("ownsActiveMotion", () => {
  it.each(["running", "starting", "paused", "completing"] as const)(
    "recognizes %s motion only for the matching source",
    (state) => {
      const snapshot = engine({ [state]: true, target: { source: "autopilot" } });

      expect(ownsActiveMotion(snapshot, "autopilot")).toBe(true);
      expect(ownsActiveMotion(snapshot, "manual_ui")).toBe(false);
    },
  );

  it("does not treat a retained target on an idle engine as active", () => {
    expect(ownsActiveMotion(engine({ target: { source: "manual_ui" } }), "manual_ui")).toBe(false);
  });
});
