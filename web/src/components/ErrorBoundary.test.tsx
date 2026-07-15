import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { stopAllAudioPlayback } from "../util/audio";
import { ErrorBoundary } from "./ErrorBoundary";

vi.mock("../api/client", () => ({
  api: { stopMotion: vi.fn() },
}));

vi.mock("../util/audio", () => ({
  stopAllAudioPlayback: vi.fn(),
}));

function BrokenApplication(): never {
  throw new Error("startup render failed");
}

describe("application ErrorBoundary", () => {
  beforeEach(() => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.mocked(api.stopMotion).mockResolvedValue({});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("retains local cleanup and Escape-to-Stop in the fatal fallback", async () => {
    const localStop = vi.fn();
    window.addEventListener("magichandy:emergency-stop", localStop, { once: true });
    render(<ErrorBoundary application><BrokenApplication /></ErrorBoundary>);

    expect(screen.getByText(/could not finish loading/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /emergency stop all motion/i })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "Escape" });

    await waitFor(() => expect(api.stopMotion).toHaveBeenCalledOnce());
    expect(stopAllAudioPlayback).toHaveBeenCalledOnce();
    expect(localStop).toHaveBeenCalledOnce();
    expect(await screen.findByRole("status")).toHaveTextContent("Stop request sent.");
  });
});
