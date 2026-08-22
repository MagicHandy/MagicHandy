import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import spanish from "../i18n/locales/es.json";
import { MotionVisualizer } from "./MotionVisualizer";

afterEach(() => setLocaleForTest("en", english));

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
              pattern_id: "four-level-circuit",
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

  it("shows dynamic geometry, anchors, variation, and decision horizon", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            target: {
              pattern_name: "Creative",
              source: "chat",
              speed_percent: 32,
              dynamic: {
                center_percent: 50,
                span_percent: 84,
                span_min_percent: 30,
                span_profile: "contrast",
                phrase_seed: 42,
                anchors: [
                  { name: "tip", position_percent: 92 },
                  { name: "middle", position_percent: 50 },
                  { name: "base", position_percent: 8 },
                ],
                variation_percent: 24,
                segment_seconds: 18,
              },
            },
          },
        }}
      />,
    );

    expect(screen.getByText("Creative")).toBeInTheDocument();
    expect(screen.getByText(/tip → middle → base · Contrast · 18s · chat/)).toBeInTheDocument();
    expect(screen.getByText("30-84%")).toBeInTheDocument();
    expect(screen.getByText("24%")).toBeInTheDocument();
    expect(screen.getByText("32%")).toBeInTheDocument();
  });

  it("summarizes a backend-authored multi-section Creative phrase honestly", () => {
		render(
			<MotionVisualizer
				motion={{
					available: true,
					engine: {
						running: true,
						paused: false,
						target: {
							source: "chat",
							speed_percent: 36,
							dynamic: {
								center_percent: 50,
								span_percent: 84,
								variation_percent: 48,
								segment_seconds: 40,
								sections: [
									{ center_percent: 50, span_percent: 84, span_min_percent: 30, variation_percent: 48, cycles: 4 },
									{ center_percent: 68, span_percent: 54, span_min_percent: 24, variation_percent: 62, cycles: 3 },
								],
							},
						},
					},
				}}
			/>,
		);

		expect(screen.getAllByText("2 sections").length).toBeGreaterThan(0);
		expect(screen.getByText(/2 sections · 40s · chat/)).toBeInTheDocument();
		expect(screen.getByText("24-84%")).toBeInTheDocument();
		expect(screen.getByText("48-62%")).toBeInTheDocument();
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
              pattern_id: "four-level-circuit",
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

  it("tracks the current playback sample through the active stroke window", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            current_sample: { position_percent: 25, time_ms: 250 },
            last_sample: { position_percent: 90, time_ms: 1500 },
            settings: {
              speed_min_percent: 10,
              speed_max_percent: 40,
              stroke_min_percent: 20,
              stroke_max_percent: 80,
              reverse_direction: false,
              apply_video_speed_limit: false,
              style: "balanced",
              handy_model: "handy_original",
            },
          },
        }}
      />,
    );

    const visualizer = screen.getByRole("img", { name: /commanded position estimate 35 percent/i });
    expect(visualizer.querySelector(".viz-device")).toHaveAttribute("data-position", "35");
  });

  it("reflects transport-boundary direction reversal in physical carriage position", () => {
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            current_sample: { position_percent: 25, time_ms: 250 },
            settings: {
              speed_min_percent: 10,
              speed_max_percent: 40,
              stroke_min_percent: 20,
              stroke_max_percent: 80,
              reverse_direction: true,
              apply_video_speed_limit: false,
              style: "balanced",
              handy_model: "handy_original",
            },
          },
        }}
      />,
    );

    const visualizer = screen.getByRole("img", { name: /commanded position estimate 65 percent/i });
    expect(visualizer.querySelector(".viz-device")).toHaveAttribute("data-position", "65");
  });

  it("localizes its computed state and accessibility description without renaming user content", () => {
    setLocaleForTest("es", spanish);
    render(
      <MotionVisualizer
        motion={{
          available: true,
          engine: {
            running: true,
            paused: false,
            target: {
              pattern_name: "Paused",
              source: "chat",
              speed_percent: 30,
            },
          },
        }}
      />,
    );

    expect(screen.getByText("En marcha")).toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /Movimiento En marcha; patrón Paused/i })).toBeInTheDocument();
  });
});
