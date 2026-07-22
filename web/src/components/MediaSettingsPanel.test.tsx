import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaScanState } from "../api/types";
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

  it("shows catalog counts, requires saving path edits, and starts only an explicit scan", async () => {
    const onChange = vi.fn();
    const result = render(<MediaSettingsPanel {...speedLimitProps} locations={["C:/media"]} savedLocations={["C:/media"]} locked={false} onChange={onChange} />);

    expect(await screen.findByText("1 videos")).toBeInTheDocument();
    result.rerender(<MediaSettingsPanel {...speedLimitProps} locations={["C:/media", "D:/new"]} savedLocations={["C:/media"]} locked={false} onChange={onChange} />);
    expect(screen.getByText("Save location changes before scanning.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Scan now" })).toBeDisabled();

    result.rerender(<MediaSettingsPanel {...speedLimitProps} locations={["C:/media", "D:/new"]} savedLocations={["C:/media", "D:/new"]} locked={false} onChange={onChange} />);
    await waitFor(() => expect(mediaVideos).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "Scan now" }));
    await waitFor(() => expect(startMediaScan).toHaveBeenCalledOnce());
  });

  it("confirms removal and exposes scan failures instead of presenting a stale success", async () => {
    mediaScan.mockResolvedValue({ scan: {
      ...completedScan,
      error: "catalog transaction failed",
      summary: { ...completedScan.summary, issues: [{ location: "C:/media", message: "folder unavailable" }] },
    } });
    const onChange = vi.fn();
    render(<MediaSettingsPanel {...speedLimitProps} locations={["C:/media"]} savedLocations={["C:/media"]} locked={false} onChange={onChange} />);

    expect(await screen.findByText("catalog transaction failed")).toHaveAttribute("role", "alert");
    expect(screen.getByText("C:/media: folder unavailable")).toHaveAttribute("role", "alert");
    fireEvent.click(screen.getByRole("button", { name: "Remove C:/media" }));
    expect(window.confirm).toHaveBeenCalledOnce();
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("keeps authored video-script speed by default and reports an opt-in change", async () => {
    render(<MediaSettingsPanel {...speedLimitProps} locations={[]} savedLocations={[]} locked={false} onChange={vi.fn()} />);
    expect(await screen.findByText("1 catalog entries")).toBeInTheDocument();

    const toggle = screen.getByRole("checkbox", { name: /Apply motion speed limit to video scripts/ });
    expect(toggle).not.toBeChecked();
    fireEvent.click(toggle);

    expect(speedLimitProps.onLimitVideoScriptSpeedChange).toHaveBeenCalledWith(true);
  });
});
