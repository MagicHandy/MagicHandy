import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MotionVisualizer } from "./MotionVisualizer";

describe("MotionVisualizer", () => {
  it("distinguishes startup from active playback", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: { running: true, starting: true, paused: false },
        }}
      />,
    );

    expect(screen.getByRole("img", { name: /motion starting/i })).toHaveAttribute("data-state", "starting");
    expect(screen.getByText("Starting")).toBeInTheDocument();
  });

  it("shows the backend-resolved active pattern name in compact telemetry", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            target: {
              pattern_id: "high-low-blocks",
              pattern_name: "High-Low Blocks",
              source: "chat",
              speed_percent: 38,
            },
          },
        }}
      />,
    );

    expect(screen.getByText("High-Low Blocks")).toBeInTheDocument();
    expect(screen.getByText("chat")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /pattern High-Low Blocks/i })).toBeInTheDocument();
  });

  it("does not present retained target metadata as currently active after Stop", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: false,
            paused: false,
            target: {
              pattern_id: "high-low-blocks",
              pattern_name: "High-Low Blocks",
              source: "chat",
              speed_percent: 38,
            },
          },
        }}
      />,
    );

    const visualizer = screen.getByRole("img", { name: /motion idle/i });
    expect(visualizer).toHaveTextContent("No active pattern");
    expect(visualizer).not.toHaveTextContent("High-Low Blocks");
    expect(Array.from(visualizer.querySelectorAll("dd"), (node) => node.textContent)).toEqual([
      "0-100%",
      "--",
      "--",
    ]);
  });

  it("shows the active video title instead of an unknown pattern", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            target: {
              label: "Paired session",
              media_id: "video-1",
              source: "media",
              speed_percent: 40,
            },
          },
        }}
      />,
    );

    expect(screen.getByText("Paired session")).toBeInTheDocument();
    expect(screen.getByText("media")).toBeInTheDocument();
  });
});
