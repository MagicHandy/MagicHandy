import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaScanState, MediaSettingsPayload } from "../api/types";
import { MediaSettingsPanel } from "./MediaSettingsPanel";

vi.mock("../api/client", () => ({
  api: {
    mediaVideos: vi.fn(),
    mediaScan: vi.fn(),
    startMediaScan: vi.fn(),
    cancelMediaScan: vi.fn(),
    pickHostPath: vi.fn(),
  },
}));

const mediaVideos = vi.mocked(api.mediaVideos);
const mediaScan = vi.mocked(api.mediaScan);
const startMediaScan = vi.mocked(api.startMediaScan);
const speedLimitProps = {
  limitVideoScriptSpeed: false,
  onLimitVideoScriptSpeedChange: vi.fn(),
};

const completedScan: MediaScanState = {
  running: false,
  cancellable: false,
  cancelled: false,
  completed_at: "2026-07-19T12:00:00Z",
  files_visited: 2,
  videos_found: 1,
  summary: { locations: 1, added: 1, updated: 0, missing: 0, removed: 0, skipped: 1, issues: [] },
};

describe("MediaSettingsPanel", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mediaVideos.mockResolvedValue({ videos: [{
      id: "one",
      location_path: "C:/media",
      display_name: "One",
      size_bytes: 100,
      modified_at: "2026-07-19T12:00:00Z",
      duration_ms: null,
      has_funscript: false,
      missing: false,
      scanned_at: "2026-07-19T12:00:00Z",
    }] });
    mediaScan.mockResolvedValue({ scan: completedScan });
    startMediaScan.mockResolvedValue({ scan: { ...completedScan, running: true, cancellable: true } });
    vi.stubGlobal("confirm", vi.fn(() => true));
  });

  afterEach(() => vi.unstubAllGlobals());

  it("reports a bounded script offset to the settings form", async () => {
    const onScriptOffsetChange = vi.fn();
    const onChange = (patch: Partial<MediaSettingsPayload>) => {
      if (patch.script_offset_ms !== undefined) onScriptOffsetChange(patch.script_offset_ms);
    };
    render(
      <MediaSettingsPanel
        {...speedLimitProps}
        media={{ library_paths: [], script_offset_ms: -150 }}
        savedMedia={{ library_paths: [] }}
        locked={false}
        onChange={onChange}
      />,
    );

    const field = await screen.findByRole("spinbutton", { name: /Script offset/ });
    expect(field).toHaveValue(-150);

    fireEvent.change(field, { target: { value: "250" } });
    expect(onScriptOffsetChange).toHaveBeenLastCalledWith(250);

    // Beyond the bound the dial stops being calibration, so the field clamps
    // instead of forwarding a value the backend would silently reduce.
    fireEvent.change(field, { target: { value: "99999" } });
    expect(onScriptOffsetChange).toHaveBeenLastCalledWith(2000);
  });

  it("shows catalog counts, requires saving scan edits, and starts a manual scan", async () => {
    const onChange = vi.fn();
    const result = render(<MediaSettingsPanel {...speedLimitProps} media={{ library_paths: ["C:/media"] }} savedMedia={{ library_paths: ["C:/media"] }} locked={false} onChange={onChange} />);

    expect(await screen.findByText("1 videos")).toBeInTheDocument();
    result.rerender(<MediaSettingsPanel {...speedLimitProps} media={{ library_paths: ["C:/media", "D:/new"] }} savedMedia={{ library_paths: ["C:/media"] }} locked={false} onChange={onChange} />);
    expect(screen.getByText("Save library or scan-option changes before scanning.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Scan now" })).toBeDisabled();

    result.rerender(<MediaSettingsPanel {...speedLimitProps} media={{ library_paths: ["C:/media", "D:/new"] }} savedMedia={{ library_paths: ["C:/media", "D:/new"] }} locked={false} onChange={onChange} />);
    await waitFor(() => expect(mediaVideos).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "Scan now" }));
    await waitFor(() => expect(startMediaScan).toHaveBeenCalledOnce());
  });

  it("consolidates scan policy and blocks a scan until option changes are saved", async () => {
    const saved: MediaSettingsPayload = { library_paths: ["C:/media"], remove_missing_on_scan: true };
    const onChange = vi.fn();
    const result = render(<MediaSettingsPanel {...speedLimitProps} media={saved} savedMedia={saved} locked={false} onChange={onChange} />);

    expect(await screen.findByRole("group", { name: "Scan options" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /Scan library when MagicHandy starts/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Remove missing catalog entries/ })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Generate missing thumbnails after scanning/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Convert unplayable files after scanning/ })).not.toBeChecked();

    fireEvent.click(screen.getByRole("checkbox", { name: /Scan library when MagicHandy starts/ }));
    expect(onChange).toHaveBeenCalledWith({ auto_scan_on_startup: true });

    result.rerender(
      <MediaSettingsPanel
        {...speedLimitProps}
        media={{ ...saved, auto_scan_on_startup: true }}
        savedMedia={saved}
        locked={false}
        onChange={onChange}
      />,
    );
    expect(screen.getByRole("button", { name: "Scan now" })).toBeDisabled();
  });

  it("confirms removal and exposes scan failures instead of presenting a stale success", async () => {
    mediaScan.mockResolvedValue({ scan: {
      ...completedScan,
      error: "catalog transaction failed",
      summary: { ...completedScan.summary, issues: [{ location: "C:/media", message: "folder unavailable" }] },
    } });
    const onChange = vi.fn();
    render(<MediaSettingsPanel {...speedLimitProps} media={{ library_paths: ["C:/media"] }} savedMedia={{ library_paths: ["C:/media"] }} locked={false} onChange={onChange} />);

    expect(await screen.findByText("catalog transaction failed")).toHaveAttribute("role", "alert");
    expect(screen.getByText("C:/media: folder unavailable")).toHaveAttribute("role", "alert");
    fireEvent.click(screen.getByRole("button", { name: "Remove C:/media" }));
    expect(window.confirm).toHaveBeenCalledOnce();
    expect(onChange).toHaveBeenCalledWith({ library_paths: [] });
  });

  it("keeps authored video-script speed by default and reports an opt-in change", async () => {
    render(<MediaSettingsPanel {...speedLimitProps} media={{ library_paths: [] }} savedMedia={{ library_paths: [] }} locked={false} onChange={vi.fn()} />);
    expect(await screen.findByText("1 catalog entries")).toBeInTheDocument();

    const toggle = screen.getByRole("checkbox", { name: /Apply motion speed limit to video scripts/ });
    expect(toggle).not.toBeChecked();
    fireEvent.click(toggle);

    expect(speedLimitProps.onLimitVideoScriptSpeedChange).toHaveBeenCalledWith(true);
  });

  it("offers the full high-quality stereo AAC target range", async () => {
    const onChange = vi.fn();
    render(
      <MediaSettingsPanel
        {...speedLimitProps}
        media={{ library_paths: [], reencode_audio_kbps: 512 }}
        savedMedia={{ library_paths: [], reencode_audio_kbps: 512 }}
        locked={false}
        onChange={onChange}
      />,
    );

    const bitrate = await screen.findByRole("slider", { name: /Audio bitrate/ });
    expect(bitrate).toHaveAttribute("min", "96");
    expect(bitrate).toHaveAttribute("max", "576");
    expect(bitrate).toHaveAttribute("step", "16");
    expect(bitrate).toHaveValue("512");

    fireEvent.change(bitrate, { target: { value: "576" } });
    expect(onChange).toHaveBeenCalledWith({ reencode_audio_kbps: 576 });
  });
});
