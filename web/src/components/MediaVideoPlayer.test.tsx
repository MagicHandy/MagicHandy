import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { MediaVideoPlayer } from "./MediaVideoPlayer";

vi.mock("../api/client", () => ({
  api: {
    mediaStreamURL: (id: string) => `/stream/${id}`,
    saveMediaDuration: vi.fn(),
    reportMediaCompatibility: vi.fn(),
    saveMediaThumbnail: vi.fn(),
  },
}));

const saveMediaDuration = vi.mocked(api.saveMediaDuration);
const reportMediaCompatibility = vi.mocked(api.reportMediaCompatibility);

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
    reportMediaCompatibility.mockReset();
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

  it("preloads synchronized media while controls are temporarily withheld", () => {
    const onPlaybackEvent = vi.fn();
    render(
      <MediaVideoPlayer
        video={video("paired")}
        allowMetadataWrite={false}
        synchronized
        controlsEnabled={false}
        busy
        onPlaybackEvent={onPlaybackEvent}
      />,
    );

    const player = screen.getByLabelText("paired") as HTMLVideoElement;
    expect(player).toHaveAttribute("preload", "auto");
    expect(player).not.toHaveAttribute("controls");
    expect(player).toHaveAttribute("tabindex", "-1");
    expect(player.closest(".media-player")).toHaveAttribute("aria-busy", "true");

    fireEvent.playing(player);
    expect(onPlaybackEvent).toHaveBeenCalledWith("playing", player);
  });

  it("does not rewrite an equivalent browser duration", () => {
    render(<MediaVideoPlayer video={video("session", 41900)} allowMetadataWrite synchronized />);
    const player = screen.getByLabelText("session") as HTMLVideoElement;
    expect(player.closest(".media-player")).toHaveAttribute("data-synchronized", "true");
    Object.defineProperty(player, "duration", { configurable: true, value: 42 });

    fireEvent.loadedMetadata(player);

    expect(saveMediaDuration).not.toHaveBeenCalled();
  });

  it("does not write from read-only playback and offers recovery from decode errors", async () => {
    const load = vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    const onPlaybackEvent = vi.fn();
    const result = render(<MediaVideoPlayer video={video("first")} allowMetadataWrite={false} onPlaybackEvent={onPlaybackEvent} />);
    const first = screen.getByLabelText("first") as HTMLVideoElement;
    Object.defineProperty(first, "duration", { configurable: true, value: 12 });
    fireEvent.loadedMetadata(first);
    fireEvent.error(first);
    expect(await screen.findByRole("alert")).toHaveTextContent("file is still available");
    expect(onPlaybackEvent).toHaveBeenCalledWith("error", first);
    expect(saveMediaDuration).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Retry video" }));
    expect(load).toHaveBeenCalledOnce();

    fireEvent.error(first);
    fireEvent.canPlay(first);
    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());

    result.rerender(<MediaVideoPlayer video={video("second")} allowMetadataWrite={false} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByLabelText("second")).toHaveAttribute("src", "/stream/second");
    load.mockRestore();
  });

  // The case the whole feature exists for: a container every browser opens,
  // holding a codec this one cannot decode. Nothing about the filename says so,
  // and only the element's own error code reveals it.
  it("offers conversion when the browser refuses to decode a reachable file", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, body: null });
    vi.stubGlobal("fetch", fetchMock);
    reportMediaCompatibility.mockResolvedValue({ status: "saved" });
    const onRequestConversion = vi.fn();

    render(
      <MediaVideoPlayer
        video={video("hevc")}
        allowMetadataWrite
        onRequestConversion={onRequestConversion}
      />,
    );
    const player = screen.getByLabelText("hevc") as HTMLVideoElement;
    // MEDIA_ERR_SRC_NOT_SUPPORTED: the bytes arrived and were refused.
    Object.defineProperty(player, "error", { configurable: true, value: { code: 4 } });
    fireEvent.error(player);

    expect(await screen.findByText("This browser cannot play this file")).toBeInTheDocument();
    await waitFor(() => expect(reportMediaCompatibility).toHaveBeenCalledWith("hevc", "unsupported_codec"));
    fireEvent.click(screen.getByRole("button", { name: "Convert this video" }));
    expect(onRequestConversion).toHaveBeenCalledOnce();
    vi.unstubAllGlobals();
  });

  // A deleted file also raises code 4 in some browsers. Blaming the codec there
  // would send the user to convert a file that is simply gone.
  it("does not blame the codec when the file cannot be fetched", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, body: null });
    vi.stubGlobal("fetch", fetchMock);

    render(<MediaVideoPlayer video={video("gone")} allowMetadataWrite onRequestConversion={vi.fn()} />);
    const player = screen.getByLabelText("gone") as HTMLVideoElement;
    Object.defineProperty(player, "error", { configurable: true, value: { code: 4 } });
    fireEvent.error(player);

    expect(await screen.findByRole("alert")).toHaveTextContent("file is still available");
    expect(screen.queryByRole("button", { name: "Convert this video" })).not.toBeInTheDocument();
    expect(reportMediaCompatibility).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it("records a successful decode so the repair offer does not persist", async () => {
    reportMediaCompatibility.mockResolvedValue({ status: "saved" });
    render(<MediaVideoPlayer video={{ ...video("mixed"), compatibility: "unsupported_codec" }} allowMetadataWrite />);
    fireEvent.canPlay(screen.getByLabelText("mixed"));
    await waitFor(() => expect(reportMediaCompatibility).toHaveBeenCalledWith("mixed", "playable"));
  });
});
