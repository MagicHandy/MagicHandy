import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { PresetModesRoute } from "./PresetModesRoute";

const app = vi.hoisted(() => ({
  readOnly: false,
  state: {
    modes: {} as { mode?: string; segment_index?: number; decision_source?: string; last_say?: string },
    settings: { motion: { style: "balanced" } },
  },
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    startMode: vi.fn(),
    stopMode: vi.fn(),
    applyQuick: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    state: app.state,
    backendOnline: true,
    readOnly: app.readOnly,
    motion: { engine: { paused: false } },
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

const startMode = vi.mocked(api.startMode);
const applyQuick = vi.mocked(api.applyQuick);

describe("PresetModesRoute", () => {
  beforeEach(() => {
    app.readOnly = false;
    app.state = { modes: {}, settings: { motion: { style: "balanced" } } };
    app.refresh.mockReset();
    app.show.mockReset();
    startMode.mockReset();
    vi.mocked(api.stopMode).mockReset();
    applyQuick.mockReset();
  });

  it("deduplicates rapid mode starts before React can disable the control", async () => {
    let release!: (value: unknown) => void;
    startMode.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    render(<PresetModesRoute />);
    const start = screen.getByRole("button", { name: "Start Freestyle" });

    act(() => {
      start.click();
      start.click();
    });

    expect(startMode).toHaveBeenCalledOnce();
    await act(async () => release({}));
    await waitFor(() => expect(start).toBeEnabled());
  });

  it("serializes style writes and removes internal phase language", async () => {
    let release!: () => void;
    applyQuick.mockImplementation(() => new Promise<void>((resolve) => { release = resolve; }));
    render(<PresetModesRoute />);
    const intense = screen.getByRole("button", { name: "Intense" });

    act(() => {
      intense.click();
      intense.click();
    });

    expect(applyQuick).toHaveBeenCalledOnce();
    expect(screen.queryByText(/Phase 11|Phase 14/)).not.toBeInTheDocument();
    await act(async () => release());
  });

  it("keeps mode-specific Stop unavailable to read-only clients", () => {
    app.readOnly = true;
    app.state = { modes: { mode: "freestyle" }, settings: { motion: { style: "balanced" } } };
    render(<PresetModesRoute />);

    expect(screen.getByRole("button", { name: "Stop Freestyle" })).toBeDisabled();
  });

  it("starts Autopilot through the modes endpoint", async () => {
    startMode.mockResolvedValue({});
    render(<PresetModesRoute />);

    await act(async () => screen.getByRole("button", { name: "Start Autopilot" }).click());

    expect(startMode).toHaveBeenCalledWith("autopilot");
  });

  it("shows Autopilot decision provenance and the last spoken line", () => {
    app.state = {
      modes: { mode: "autopilot", segment_index: 4, decision_source: "fallback", last_say: "Keeping it steady." },
      settings: { motion: { style: "balanced" } },
    };
    render(<PresetModesRoute />);

    expect(screen.getByRole("button", { name: "Stop Autopilot" })).toBeEnabled();
    expect(screen.getByRole("status")).toHaveTextContent("Segment 4 — Deterministic fallback (model unavailable)");
    expect(screen.getByText("“Keeping it steady.”")).toBeInTheDocument();
    // Freestyle stays startable copy-wise but the autopilot card owns the stop.
    expect(screen.getByRole("button", { name: "Start Freestyle" })).toBeInTheDocument();
  });
});
