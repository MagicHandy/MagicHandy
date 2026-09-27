import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { videoIDFromRoute, videoRoute } from "../videos/route";
import { VideoRoute } from "./VideoRoute";

const app = vi.hoisted(() => ({ backendOnline: true, readOnly: false, state: { stop_sequence: 7 } }));

vi.mock("../state/app-state", async () => {
  const { useEffect, useState } = await import("react");
  return {
    useAppState: () => app,
    useHashRoute: () => {
      const [hash, setHash] = useState(() => window.location.hash || "#/chat");
      useEffect(() => {
        const onChange = () => setHash(window.location.hash || "#/chat");
        window.addEventListener("hashchange", onChange);
        return () => window.removeEventListener("hashchange", onChange);
      }, []);
      return hash;
    },
  };
});

vi.mock("../components/VideoLibrary", () => ({
  VideoLibrary: ({ locked, stopSequence, selectedID, onSelect }: { locked: boolean; stopSequence?: number; selectedID?: string; onSelect?: (id: string) => void }) => (
    <div data-testid="video-catalog" data-locked={locked} data-stop-sequence={stopSequence} data-selected={selectedID}>
      <button type="button" onClick={() => onSelect?.("clip one")}>Open</button>
    </div>
  ),
}));

describe("VideoRoute", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
    window.location.hash = "#/videos";
  });

  afterEach(() => {
    window.location.hash = "";
  });

  it("renders Videos as a dedicated wide workspace", () => {
    render(<VideoRoute />);

    expect(screen.getByRole("heading", { level: 1, name: "Videos" })).toHaveFocus();
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-locked", "false");
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-stop-sequence", "7");
    expect(screen.getByTestId("video-catalog").parentElement).toHaveClass("video-page");
  });

  it("locks catalog mutations for an offline or read-only client", () => {
    app.readOnly = true;
    render(<VideoRoute />);

    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-locked", "true");
  });

  it("opens the video named by the route and records selections in it", async () => {
    window.location.hash = "#/videos/alpha";
    render(<VideoRoute />);
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-selected", "alpha");

    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(window.location.hash).toBe("#/videos/clip%20one");
    await act(async () => window.dispatchEvent(new HashChangeEvent("hashchange")));
    expect(screen.getByTestId("video-catalog")).toHaveAttribute("data-selected", "clip one");
  });

  it("parses and builds video routes", () => {
    expect(videoIDFromRoute("#/videos")).toBe("");
    expect(videoIDFromRoute("#/videos/abc123")).toBe("abc123");
    expect(videoIDFromRoute("#/chat/abc123")).toBe("");
    expect(videoRoute("")).toBe("#/videos");
    expect(videoRoute("a b")).toBe("#/videos/a%20b");
  });
});
