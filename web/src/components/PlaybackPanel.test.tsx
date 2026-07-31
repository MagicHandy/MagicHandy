import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { PlaybackPanel } from "./PlaybackPanel";

vi.mock("../api/client", () => ({
  api: { saveMediaScriptOffset: vi.fn(), saveMediaPlayback: vi.fn() },
}));

const saveOffset = vi.mocked(api.saveMediaScriptOffset);
const savePlayback = vi.mocked(api.saveMediaPlayback);

function video(offsetMillis = 0): MediaVideo {
  return {
    id: "clip", location_path: "C:/media", display_name: "beach.mp4",
    size_bytes: 1, modified_at: "now", duration_ms: 60_000,
    has_funscript: true, missing: false, scanned_at: "now",
    script_offset_ms: offsetMillis,
  };
}

function panel(props: Partial<Parameters<typeof PlaybackPanel>[0]> = {}) {
  return (
    <PlaybackPanel
      video={video(props.video?.script_offset_ms ?? -70)}
      sync={{ active: true, state: "following" }}
      locked={false}
      setupOffsetMillis={-150}
      smoothingPercent={0}
      roundingMillis={0}
      limitSpeed={false}
      speedLimitPercent={40}
      onClose={vi.fn()}
      {...props}
    />
  );
}

describe("PlaybackPanel", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    saveOffset.mockReset();
    savePlayback.mockReset();
    saveOffset.mockResolvedValue(undefined as never);
    savePlayback.mockResolvedValue(undefined as never);
  });

  it("shows the effective offset as this video plus the setup value", () => {
    render(panel());
    // −70 for the file plus −150 for the room. Showing only one of them would
    // make a surprising total unexplainable.
    expect(screen.getByText("−220 ms")).toBeInTheDocument();
    expect(screen.getByText(/this video −70 ms · setup −150 ms/)).toBeInTheDocument();
  });

  it("writes the per-video offset once per gesture", async () => {
    render(panel());
    const slider = screen.getByRole("slider", { name: /Sync offset/ });

    fireEvent.change(slider, { target: { value: "-100" } });
    fireEvent.change(slider, { target: { value: "-40" } });
    fireEvent.change(slider, { target: { value: "20" } });
    expect(saveOffset).not.toHaveBeenCalled();

    await act(() => vi.advanceTimersByTimeAsync(200));
    expect(saveOffset).toHaveBeenCalledOnce();
    expect(saveOffset).toHaveBeenCalledWith("clip", 20);
  });

  it("reveals a filter's own control only once it is on", async () => {
    render(panel());
    expect(screen.queryByRole("slider", { name: /Peak rounding/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: /Round peaks/ }));
    await act(() => vi.advanceTimersByTimeAsync(200));
    expect(savePlayback).toHaveBeenCalledWith({ peak_rounding_ms: 60 });
    expect(screen.getByRole("slider", { name: /Peak rounding/ })).toBeInTheDocument();
  });

  it("states that nothing is filtered when nothing is", () => {
    render(panel());
    expect(screen.getByText("Playing the script exactly as authored.")).toBeInTheDocument();
  });

  it("keeps an empty filter report in the pending state", () => {
    render(panel({
      smoothingPercent: 3,
      sync: { active: true, state: "following", filter_effect: {} },
    }));
    expect(screen.getByText("Filters on; effect is measured when motion re-arms.")).toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });

  it("names the configured maximum when the video speed cap is on", () => {
    render(panel({ limitSpeed: true, speedLimitPercent: 35 }));
    expect(screen.getByText("35% max")).toBeInTheDocument();
    expect(screen.getByText("Travel is capped at 35% without changing the video clock.")).toBeInTheDocument();
  });

  it("reports what the filters measurably changed", () => {
    render(panel({
      smoothingPercent: 3,
      sync: {
        active: true, state: "following",
        filter_effect: { actions_removed: 214, peak_reduction_percent: 2.1 },
      },
    }));
    expect(screen.getByText(/214 actions removed · peaks up to 2.1% lower/)).toBeInTheDocument();
  });

  it("disables every control for a read-only tab", () => {
    render(panel({ locked: true }));
    expect(screen.getByRole("slider", { name: /Sync offset/ })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: /Smoothing/ })).toBeDisabled();
    expect(screen.getByText(/Read-only tab/)).toBeInTheDocument();
  });
});
