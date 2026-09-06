import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaPlaybackSettings, MediaVideo } from "../api/types";
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
    savePlayback.mockImplementation(async (patch) => ({
      media: {script_smoothing_percent: patch.script_smoothing_percent ?? 0, peak_rounding_ms: patch.peak_rounding_ms ?? 0},
      motion: {apply_video_speed_limit: patch.apply_video_speed_limit ?? false},
    }));
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
    expect(screen.getByText("Filters off; authored motion is preserved.")).toBeInTheDocument();
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
    render(panel({ limitSpeed: true, speedLimitPercent: 35, sync: {active:true,state:"following",filter_effect:{smoothing_percent:0,rounding_ms:0,speed_limit_enabled:true}} }));
    expect(screen.getByText("35% max")).toBeInTheDocument();
    expect(screen.getByText("Travel is capped at 35% without changing the video clock.")).toBeInTheDocument();
  });

  it("reports what the filters measurably changed", () => {
    render(panel({
      smoothingPercent: 3,
      sync: {
        active: true, state: "following",
        filter_effect: { smoothing_percent:3,rounding_ms:0,speed_limit_enabled:false,actions_removed: 214, peak_reduction_percent: 2.1 },
      },
    }));
    expect(screen.getByText(/214 actions removed · Peaks up to 2.1% lower/)).toBeInTheDocument();
  });

  it("disables every control for a read-only tab", () => {
    render(panel({ locked: true }));
    expect(screen.getByRole("slider", { name: /Sync offset/ })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: /Smoothing/ })).toBeDisabled();
    expect(screen.getByText(/Read-only tab/)).toBeInTheDocument();
  });

  it("distinguishes measured zero from stale filter results", () => {
    const sync = { active: true, state: "following" as const, filter_effect: { smoothing_percent: 3, rounding_ms: 0, speed_limit_enabled: false } };
    const view = render(panel({ smoothingPercent: 3, sync }));
    expect(screen.getByText("No eligible script changes at these settings.")).toBeInTheDocument();
    view.rerender(panel({ smoothingPercent: 4, sync }));
    expect(screen.getByText("Filters on; effect is measured when motion re-arms.")).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: /Smoothing/ })).toHaveValue("4");
  });

  it("reports local timing and windows that could not honor the requested rounding", () => {
    render(panel({ roundingMillis: 200, sync: { active: true, state: "following", filter_effect: {
      smoothing_percent: 0, rounding_ms: 200, speed_limit_enabled: false,
      rounded_corners: 8, peak_shift_ms: 23.4, rounding_limited_corners: 4, rounding_skipped_corners: 2,
    } } }));
    expect(screen.getByText("8 corners rounded · Peak timing shifts up to 23.4 ms · 4 corners use shorter windows · 2 corners too short to round")).toBeInTheDocument();
  });

  it("serializes writes and preserves a newer draft until its backend readback arrives", async () => {
    let finishFirst!: (saved: MediaPlaybackSettings) => void;
    let finishSecond!: (saved: MediaPlaybackSettings) => void;
    savePlayback.mockReturnValueOnce(new Promise((resolve) => { finishFirst = resolve; }))
      .mockReturnValueOnce(new Promise((resolve) => { finishSecond = resolve; }));
    render(panel());
    fireEvent.click(screen.getByRole("checkbox", { name: /Round peaks/ }));
    await act(() => vi.advanceTimersByTimeAsync(200));
    fireEvent.change(screen.getByRole("slider", { name: /Peak rounding/ }), { target: { value: "120" } });
    await act(() => vi.advanceTimersByTimeAsync(200));
    expect(savePlayback).toHaveBeenCalledOnce();
    await act(async () => finishFirst({ media: { script_smoothing_percent: 0, peak_rounding_ms: 60 }, motion: { apply_video_speed_limit: false } }));
    expect(savePlayback).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("slider", { name: /Peak rounding/ })).toHaveValue("120");
    expect(screen.getByText("Filter changes are being applied.")).toBeInTheDocument();
    await act(async () => finishSecond({ media: { script_smoothing_percent: 0, peak_rounding_ms: 110 }, motion: { apply_video_speed_limit: false } }));
    expect(screen.getByRole("slider", { name: /Peak rounding/ })).toHaveValue("110");
    expect(screen.queryByText("Filter changes are being applied.")).not.toBeInTheDocument();
  });

  it("rolls a failed write back to the latest backend snapshot", async () => {
    let reject!: (reason: Error) => void;
    savePlayback.mockReturnValueOnce(new Promise((_, fail) => { reject = fail; }));
    const view = render(panel());
    fireEvent.click(screen.getByRole("checkbox", { name: /Round peaks/ }));
    await act(() => vi.advanceTimersByTimeAsync(200));
    view.rerender(panel({ smoothingPercent: 2, roundingMillis: 20 }));
    await act(async () => reject(new Error("Settings write failed")));
    expect(screen.getByRole("slider", { name: /Peak rounding/ })).toHaveValue("20");
    expect(screen.getByRole("slider", { name: /Smoothing/ })).toHaveValue("2");
    expect(screen.getByText("Settings write failed")).toBeInTheDocument();
  });
});
