import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MediaFunscript } from "../api/types";
import { FunscriptTimeline, intensityBand } from "./FunscriptTimeline";

const script: MediaFunscript = {
  video_id: "video-1",
  name: "Session",
  duration_ms: 10_000,
  action_count: 4,
  actions: [
    { at: 0, pos: 20 },
    { at: 2_000, pos: 80 },
    { at: 5_000, pos: 35 },
    { at: 10_000, pos: 65 },
  ],
};

describe("FunscriptTimeline", () => {
  it("classifies authored segment speed without flattening high-intensity motion", () => {
    expect(intensityBand(50)).toBe("idle");
    expect(intensityBand(51)).toBe("moderate");
    expect(intensityBand(201)).toBe("fast");
    expect(intensityBand(401)).toBe("very-fast");
  });

  it("renders an accessible timeline and supports pointer and keyboard seeking", () => {
    const onSeek = vi.fn();
    render(<FunscriptTimeline script={script} currentTime={2_000} hidden={false} onSeek={onSeek} />);

    const timeline = screen.getByRole("slider", { name: /funscript timeline/i });
    const canvas = timeline.querySelector("canvas");
    expect(canvas).not.toBeNull();
    Object.defineProperty(canvas as HTMLCanvasElement, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ left: 10, width: 200, top: 0, right: 210, bottom: 88, height: 88, x: 10, y: 0, toJSON() {} }),
    });
    fireEvent.pointerDown(timeline, { button: 0, clientX: 110, pointerId: 1 });
    expect(onSeek).toHaveBeenLastCalledWith(5_000);

    fireEvent.keyDown(timeline, { key: "ArrowRight" });
    expect(onSeek).toHaveBeenLastCalledWith(7_000);
    fireEvent.keyDown(timeline, { key: "End" });
    expect(onSeek).toHaveBeenLastCalledWith(10_000);
  });

  it("keeps a compact progress indicator when the full curve is hidden", () => {
    render(<FunscriptTimeline script={script} currentTime={2_500} hidden onSeek={vi.fn()} />);

    const progress = screen.getByLabelText("Funscript progress 00:02.500 of 00:10");
    expect(progress.firstElementChild).toHaveStyle({ width: "25%" });
    expect(screen.queryByRole("slider")).not.toBeInTheDocument();
  });

  it("renders the documented 100,000-action bound without expanding the DOM", () => {
    const actions = Array.from({ length: 100_000 }, (_, index) => ({
      at: index * 10,
      pos: index % 101,
    }));
    const featureScript: MediaFunscript = {
      video_id: "feature",
      name: "Feature length",
      duration_ms: actions[actions.length - 1].at,
      action_count: actions.length,
      actions,
    };

    render(<FunscriptTimeline script={featureScript} currentTime={0} hidden={false} onSeek={vi.fn()} />);

    const timeline = screen.getByRole("slider", { name: "Funscript timeline, 100,000 actions" });
    expect(timeline.querySelectorAll("canvas")).toHaveLength(2);
    expect(timeline.childElementCount).toBe(2);
  });
});
