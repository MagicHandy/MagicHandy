import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { MediaVideoPlayer } from "./MediaVideoPlayer";

vi.mock("../api/client", () => ({
  api: {
    mediaStreamURL: (id: string) => `/stream/${id}`,
    saveMediaDuration: vi.fn(),
  },
}));

const saveMediaDuration = vi.mocked(api.saveMediaDuration);

function video(id: string, duration: number | null = null): MediaVideo {
  return {
    id,
    location_path: "C:/media",
    display_name: id,
    size_bytes: 1024,
    modified_at: "2026-07-19T12:00:00Z",
    duration_ms: duration,
    has_funscript: false,
    missing: false,
    scanned_at: "2026-07-19T12:00:00Z",
  };
}

describe("MediaVideoPlayer", () => {
  beforeEach(() => {
    saveMediaDuration.mockReset();
    saveMediaDuration.mockResolvedValue({ status: "saved" });
  });

  it("backfills browser-decoded duration once and reports it to the caller", async () => {
    const onDuration = vi.fn();
    const onVideoUpdate = vi.fn();
    render(<MediaVideoPlayer video={video("session")} allowMetadataWrite onDuration={onDuration} onVideoUpdate={onVideoUpdate} />);

    const player = screen.getByLabelText("session") as HTMLVideoElement;
    Object.defineProperty(player, "duration", { configurable: true, value: 42 });
    fireEvent.loadedMetadata(player);
    fireEvent.loadedMetadata(player);

    await waitFor(() => expect(saveMediaDuration).toHaveBeenCalledOnce());
    expect(saveMediaDuration).toHaveBeenCalledWith("session", 42000);
    expect(onDuration).toHaveBeenLastCalledWith(42000);
    expect(onVideoUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: "session", duration_ms: 42000 }));
  });

  it("does not rewrite an equivalent browser duration", () => {
    render(<MediaVideoPlayer video={video("session", 41900)} allowMetadataWrite />);
    const player = screen.getByLabelText("session") as HTMLVideoElement;
    Object.defineProperty(player, "duration", { configurable: true, value: 42 });

    fireEvent.loadedMetadata(player);

    expect(saveMediaDuration).not.toHaveBeenCalled();
  });

  it("does not write from read-only playback and offers recovery from decode errors", () => {
    const load = vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    const result = render(<MediaVideoPlayer video={video("first")} allowMetadataWrite={false} />);
    const first = screen.getByLabelText("first") as HTMLVideoElement;
    Object.defineProperty(first, "duration", { configurable: true, value: 12 });
    fireEvent.loadedMetadata(first);
    fireEvent.error(first);
    expect(screen.getByRole("alert")).toHaveTextContent("file still exists");
    expect(saveMediaDuration).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Retry video" }));
    expect(load).toHaveBeenCalledOnce();

    fireEvent.error(first);
    fireEvent.canPlay(first);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    result.rerender(<MediaVideoPlayer video={video("second")} allowMetadataWrite={false} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByLabelText("second")).toHaveAttribute("src", "/stream/second");
    load.mockRestore();
  });
});
