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
});
