import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { EngineSnapshot, LibraryPattern } from "../api/types";
import { PatternBrowser } from "./PatternBrowser";
import { PatternCurve } from "./PatternCurve";
import { ProgramLibrary } from "./ProgramLibrary";

const pattern: LibraryPattern = {
  id: "custom",
  name: "Custom",
  origin: "user",
  kind: "routine",
  enabled: true,
  weight: 1,
  cycle_ms: 6600,
  points: [{ time_ms: 0, position_percent: 0 }, { time_ms: 6600, position_percent: 100 }],
  preview_samples: [{ time_ms: 0, position_percent: 0 }, { time_ms: 6600, position_percent: 100 }],
  tags: [],
  created_at: "now",
  updated_at: "now",
};

describe("pattern library components", () => {
  it("reverts a weight field when the authoritative patch fails", async () => {
    const onPatch = vi.fn().mockResolvedValue(false);
    render(
      <PatternBrowser
        patterns={[pattern]}
        locked={false}
        offline={false}
        busyKeys={new Set()}
        onPatch={onPatch}
        onPlay={async () => {}}
        onFeedback={async () => {}}
        onExport={async () => {}}
        onDelete={async () => {}}
      />,
    );
    const weight = screen.getByRole("spinbutton", { name: "Weight" });

    fireEvent.change(weight, { target: { value: "2" } });
    fireEvent.blur(weight);

    await waitFor(() => expect(onPatch).toHaveBeenCalledWith("custom", { weight: 2 }));
    await waitFor(() => expect(weight).toHaveValue(1));
  });

  it("renders a backend-normalized weight after a successful patch", async () => {
    render(<NormalizedWeightBrowser />);
    const weight = screen.getByRole("spinbutton", { name: "Weight" });

    fireEvent.change(weight, { target: { value: "2" } });
    fireEvent.blur(weight);

    await waitFor(() => expect(weight).toHaveValue(1.75));
  });

  it("clamps program progress and invalid intensity limits before rendering", () => {
    const props = {
      programs: [],
      locked: false,
      offline: false,
      busyKeys: new Set<string>(),
      maxIntensity: 0,
      onImport: async () => {},
      onPlay: async () => {},
      onPause: async () => {},
      onResume: async () => {},
      onStop: async () => {},
      onExport: async () => {},
      onDelete: async () => {},
    };
    const result = render(<ProgramLibrary {...props} engine={engineWithPhase(1.7)} />);

    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
    expect(screen.getByRole("progressbar").firstElementChild).toHaveStyle({ width: "100%" });
    expect(screen.getByRole("slider", { name: /Intensity/ })).toHaveAttribute("max", "1");

    result.rerender(<ProgramLibrary {...props} engine={engineWithPhase(-0.4)} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");
    expect(screen.getByRole("progressbar").firstElementChild).toHaveStyle({ width: "0%" });
  });

  it("does not emit invalid SVG coordinates for malformed samples", () => {
    const result = render(<PatternCurve label="Curve" points={[
      { time_ms: Number.NaN, position_percent: 20 },
      { time_ms: 20, position_percent: -50 },
      { time_ms: 10, position_percent: 160 },
    ]} />);

    const path = result.container.querySelector("path");
    expect(path?.getAttribute("d")).not.toContain("NaN");
    expect(path?.getAttribute("d")).toBe("M120.00 5.00 L235.00 67.00");
  });
});

function engineWithPhase(phase: number): EngineSnapshot {
  return {
    target: { kind: "program", label: "Demo", program_id: "demo" },
    phase,
    running: true,
    paused: false,
    starting: false,
    completing: false,
    running_ms: 100,
  } as EngineSnapshot;
}

function NormalizedWeightBrowser() {
  const [current, setCurrent] = useState(pattern);
  return (
    <PatternBrowser
      patterns={[current]}
      locked={false}
      offline={false}
      busyKeys={new Set()}
      onPatch={async () => {
        setCurrent((value) => ({ ...value, weight: 1.75 }));
        return true;
      }}
      onPlay={async () => {}}
      onFeedback={async () => {}}
      onExport={async () => {}}
      onDelete={async () => {}}
    />
  );
}
