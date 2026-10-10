import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { MotionInfo } from "../../api/types";
import { MotionVisualizer } from "../MotionVisualizer";
import { DISABLED_VISUALIZER_KINDS, handyTravelMillimetres, visualizerKind, type VisualizerKind } from "./device";
import { dotText, dotTextSpan } from "./dotFont";

afterEach(cleanup);

function running(model = "handy_2_pro", position = 50): MotionInfo {
  return {
    available: true,
    engine: {
      running: true,
      paused: false,
      current_sample: { position_percent: position, time_ms: 1000 },
      settings: { speed_min_percent: 10, speed_max_percent: 90, stroke_min_percent: 20, stroke_max_percent: 80, reverse_direction: false, apply_video_speed_limit: false, style: "balanced", handy_model: model },
      target: { source: "chat", pattern_name: "Stroke", speed_percent: 40 },
    },
  } as MotionInfo;
}

describe("visualizer device selection", () => {
  it("follows the dispatch owner, then the Handy model", () => {
    expect(visualizerKind({ owner: "intiface", model: "handy_original" })).toBe("scan");
    expect(visualizerKind({ owner: "cloud_rest", model: "handy_original" })).toBe("handy-original");
    expect(visualizerKind({ owner: "browser_bluetooth", model: "handy_2_standard" })).toBe("handy-dots");
    expect(visualizerKind({ owner: "cloud_rest", model: "handy_2_pro" })).toBe("handy-dots");
    expect(visualizerKind(undefined)).toBe("handy-dots");
  });

  it("never selects a published-but-disabled design on its own", () => {
    const owners = ["cloud_rest", "browser_bluetooth", "intiface", "", undefined];
    const models = ["handy_original", "handy_2_standard", "handy_2_pro", "", undefined];
    for (const owner of owners) {
      for (const model of models) {
        expect(DISABLED_VISUALIZER_KINDS).not.toContain(visualizerKind({ owner, model }));
      }
    }
  });

  it("reads each model's full travel", () => {
    expect(handyTravelMillimetres("handy_original")).toBe(110);
    expect(handyTravelMillimetres("handy_2_standard")).toBe(125);
    expect(handyTravelMillimetres("handy_2_pro")).toBe(125);
  });
});

describe("dot font", () => {
  it("centres a run of digits on the requested point", () => {
    for (const text of ["7", "62", "100"]) {
      const xs = dotText(text, 80, 0, 4).map((dot) => dot.x);
      expect((Math.min(...xs) + Math.max(...xs)) / 2).toBeCloseTo(80);
      expect(Math.max(...xs) - Math.min(...xs)).toBeCloseTo(dotTextSpan(text) * 4);
    }
  });
});

describe("visualizer drawings", () => {
  it("shows the Handy 2 dot display with millimetres of 125 mm travel", () => {
    const { container } = render(<MotionVisualizer motion={running("handy_2_pro", 50)} device={{ owner: "browser_bluetooth" }} />);
    const visualizer = screen.getByRole("img", { name: /commanded position estimate 50 percent/i });
    expect(visualizer).toHaveAttribute("data-kind", "handy-dots");
    expect(screen.getByText("63 mm")).toBeInTheDocument();
    expect(container.querySelectorAll(".viz-carriage circle").length).toBeGreaterThan(0);
    expect(container.querySelectorAll(".viz-stroke-range circle").length).toBeGreaterThan(0);
  });

  it("shows the refined original front with millimetres of 110 mm travel", () => {
    render(<MotionVisualizer motion={running("handy_original", 50)} device={{ owner: "cloud_rest" }} />);
    expect(screen.getByRole("img", { name: /motion running/i })).toHaveAttribute("data-kind", "handy-original");
    expect(screen.getByText("55 mm")).toBeInTheDocument();
  });

  it("shows the scanning bar for Intiface, in percent only", () => {
    const { container } = render(<MotionVisualizer motion={running("handy_2_pro", 50)} device={{ owner: "intiface" }} />);
    expect(screen.getByRole("img", { name: /motion running/i })).toHaveAttribute("data-kind", "scan");
    expect(container.querySelector(".viz-scan-bar")).toBeInTheDocument();
    expect(screen.queryByText(/ mm$/)).not.toBeInTheDocument();
  });

  it("hides the stroke window and lit position while idle", () => {
    const idle = { available: true, engine: { running: false, paused: false } } as MotionInfo;
    const { container } = render(<MotionVisualizer motion={idle} device={{ owner: "cloud_rest", model: "handy_2_pro" }} />);
    expect(container.querySelectorAll(".viz-stroke-range circle")).toHaveLength(0);
    expect(container.querySelectorAll(".viz-carriage circle")).toHaveLength(0);
    // The dot readout shows dashes instead of a number.
    expect(container.querySelectorAll(".viz-digit-on").length).toBe(10);
  });

  it.each<VisualizerKind>(["handy-dots", "handy-original", "scan", "handy-front", "ribbon"])("renders the %s drawing in both forms", (kind) => {
    const { container, unmount } = render(<MotionVisualizer motion={running()} kind={kind} />);
    const device = container.querySelector(".viz-device");
    expect(device).toHaveAttribute("data-position", "50");
    expect(container.querySelector(".viz-track")).toBeInTheDocument();
    expect(container.querySelector(".viz-carriage")).toBeInTheDocument();
    unmount();
    const mini = render(<MotionVisualizer motion={running()} kind={kind} mini />);
    expect(mini.container.querySelector(".visualizer.mini .viz-device")).toBeInTheDocument();
  });

  it("keeps the original Handy's status-bar drawing inside its own shapes", () => {
    const { container } = render(<MotionVisualizer motion={running("handy_original")} device={{ owner: "cloud_rest" }} mini />);
    // The travel line belongs to the detailed form; at 28 px it ran past the sleeve.
    expect(container.querySelector(".viz-track")).not.toBeInTheDocument();
    expect(container.querySelector(".viz-carriage .viz-sleeve")).toBeInTheDocument();
  });
});
