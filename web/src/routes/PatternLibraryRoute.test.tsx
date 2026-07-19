import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LibraryPattern, PatternFeedback, PatternLibrary } from "../api/types";
import { PatternLibraryRoute } from "./PatternLibraryRoute";

const app = vi.hoisted(() => ({
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    getLibrary: vi.fn(),
    patchPattern: vi.fn(),
    playPattern: vi.fn(),
    patternFeedback: vi.fn(),
    undoPatternFeedback: vi.fn(),
    createPattern: vi.fn(),
    previewPattern: vi.fn(),
    importMotionContent: vi.fn(),
    deletePattern: vi.fn(),
    deleteProgram: vi.fn(),
    exportPattern: vi.fn(),
    exportProgram: vi.fn(),
    playProgram: vi.fn(),
    pauseMotion: vi.fn(),
    resumeMotion: vi.fn(),
    stopMotion: vi.fn(),
    setPatternAutoDisable: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    backendOnline: true,
    readOnly: false,
    motion: null,
    state: { settings: { motion: { speed_max_percent: 40 } } },
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

const stroke: LibraryPattern = {
  id: "stroke",
  name: "Stroke",
  description: "Even reversals.",
  origin: "builtin",
  kind: "routine",
  enabled: true,
  weight: 1,
  cycle_ms: 6600,
  points: [{ time_ms: 0, position_percent: 0 }, { time_ms: 6600, position_percent: 100 }],
  preview_samples: [{ time_ms: 0, position_percent: 0 }, { time_ms: 6600, position_percent: 100 }],
  tags: ["steady"],
  created_at: "now",
  updated_at: "now",
};

const pulse: LibraryPattern = { ...stroke, id: "pulse", name: "Pulse", origin: "user", enabled: false };

const library: PatternLibrary = {
  patterns: [stroke, pulse],
  programs: [{
    id: "demo",
    name: "Demo program",
    origin: "imported",
    duration_ms: 10000,
    points: stroke.points,
    preview_samples: stroke.preview_samples,
    created_at: "now",
    updated_at: "now",
  }],
  feedback: [],
  auto_disable: false,
};

const getLibrary = vi.mocked(api.getLibrary);
const patternFeedback = vi.mocked(api.patternFeedback);
const importMotionContent = vi.mocked(api.importMotionContent);

describe("PatternLibraryRoute", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    getLibrary.mockResolvedValue({ library });
  });

  it("shows a recoverable storage error instead of a false empty catalog", async () => {
    getLibrary.mockRejectedValueOnce(new Error("pattern store unavailable"));
    render(<PatternLibraryRoute />);

    expect(await screen.findByRole("alert")).toHaveTextContent("pattern store unavailable");
    expect(screen.queryByText("No matching patterns")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("heading", { level: 3, name: "Stroke" })).toBeInTheDocument();
    expect(getLibrary).toHaveBeenCalledTimes(2);
  });

  it("keeps author drafts mounted and supports roving tab keyboard focus", async () => {
    render(<PatternLibraryRoute />);
    await screen.findByRole("heading", { level: 3, name: "Stroke" });

    const browse = screen.getByRole("tab", { name: "Browse" });
    browse.focus();
    fireEvent.keyDown(browse, { key: "ArrowRight" });
    expect(screen.getByRole("tab", { name: "Programs" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Programs" })).toHaveFocus();

    fireEvent.click(screen.getByRole("tab", { name: "Author" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Unsaved curve" } });
    fireEvent.click(screen.getByRole("tab", { name: "Training" }));
    fireEvent.click(screen.getByRole("tab", { name: "Author" }));

    expect(screen.getByLabelText("Name")).toHaveValue("Unsaved curve");
  });

  it("imports a trimmed funscript from the Import tab and lands on the result", async () => {
    const imported = {
      ...library.programs[0],
      id: "imported",
      name: "Imported program",
    };
    importMotionContent.mockResolvedValue({
      import: { kind: "program", program: imported, gaps_stripped: 0 },
    });
    render(<PatternLibraryRoute />);
    await screen.findByRole("heading", { level: 3, name: "Stroke" });
    fireEvent.click(screen.getByRole("tab", { name: "Import" }));

    const fileInput = screen.getByRole("region", { name: "Import motion content" }).querySelector("input[type=file]");
    expect(fileInput).not.toBeNull();
    const funscript = JSON.stringify({ actions: [{ at: 0, pos: 0 }, { at: 1000, pos: 100 }, { at: 2000, pos: 10 }] });
    fireEvent.change(fileInput as HTMLInputElement, {
      target: { files: [new File([funscript], "imported.funscript", { type: "application/json" })] },
    });

    expect(await screen.findByRole("group", { name: /funscript timeline editor/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Import as program" }));

    // The authoritative response lands without a catalog reload, and the view
    // switches to Programs where the imported row is visible.
    expect(await screen.findByRole("heading", { level: 3, name: "Imported program" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Programs" })).toHaveAttribute("aria-selected", "true");
    expect(importMotionContent).toHaveBeenCalledTimes(1);
    expect(importMotionContent.mock.calls[0][1]).toBe("program");
    expect(getLibrary).toHaveBeenCalledOnce();
  });

  it("deduplicates one pattern mutation without hiding another in-flight mutation", async () => {
    const first = deferred<{ feedback: PatternFeedback; pattern: LibraryPattern }>();
    const second = deferred<{ feedback: PatternFeedback; pattern: LibraryPattern }>();
    patternFeedback.mockImplementation((id) => id === "stroke" ? first.promise : second.promise);
    render(<PatternLibraryRoute />);
    await screen.findByRole("heading", { level: 3, name: "Stroke" });

    const strokeUp = screen.getByRole("button", { name: "Rate Stroke up" });
    const pulseUp = screen.getByRole("button", { name: "Rate Pulse up" });
    fireEvent.click(strokeUp);
    fireEvent.click(strokeUp);
    fireEvent.click(pulseUp);

    expect(patternFeedback).toHaveBeenCalledTimes(2);
    expect(strokeUp).toBeDisabled();
    expect(pulseUp).toBeDisabled();

    await act(async () => first.resolve(feedbackResult(stroke, 1)));
    await waitFor(() => expect(screen.getByRole("button", { name: "Rate Stroke up" })).toBeEnabled());
    expect(screen.getByRole("button", { name: "Rate Pulse up" })).toBeDisabled();

    await act(async () => second.resolve(feedbackResult(pulse, 2)));
    await waitFor(() => expect(screen.getByRole("button", { name: "Rate Pulse up" })).toBeEnabled());
  });
});

function feedbackResult(pattern: LibraryPattern, id: number) {
  const next = { ...pattern, weight: pattern.weight + 0.1 };
  return {
    pattern: next,
    feedback: {
      id,
      pattern_id: pattern.id,
      rating: 1 as const,
      weight_before: pattern.weight,
      weight_after: next.weight,
      enabled_before: pattern.enabled,
      enabled_after: pattern.enabled,
      reverted: false,
      created_at: "now",
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}
