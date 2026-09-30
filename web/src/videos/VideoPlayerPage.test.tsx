import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect, useMemo } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import type { VideoPlayerHandle } from "../media/playbackController";
import { VideoPlayerPage } from "./VideoPlayerPage";

const app = vi.hoisted(() => ({ modesRunning: false, mediaID: "" }));

vi.mock("../api/client", () => ({ api: { getChatSessions: vi.fn(), stopMode: vi.fn() } }));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: {
    chat: { active_session_id: "session-2" }, modes: { running: app.modesRunning },
    motion: { engine: { running: Boolean(app.mediaID), target: { media_id: app.mediaID } } },
  } }),
  useToast: () => ({ show: vi.fn() }),
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: ({ motionOwner }: { motionOwner?: string }) => <div data-testid="chat-panel" data-motion-owner={motionOwner ?? "chat"} />,
}));
vi.mock("../components/SyncedVideoPlayer", () => ({
  SyncedVideoPlayer: ({ synchronized, onHandleChange, video, stopSequence }: { synchronized: boolean; onHandleChange: (handle: VideoPlayerHandle | null) => void; video: MediaVideo; stopSequence: number }) => {
    const handle = useMemo(() => ({ videoID: video.id, synchronized, getStopSequence: () => stopSequence, releaseMotion, commands: { pause, play } }) as unknown as VideoPlayerHandle, [video.id, synchronized, stopSequence]);
    useEffect(() => { onHandleChange(handle); return () => onHandleChange(null); }, [handle, onHandleChange]);
    return <div data-testid="player" data-synchronized={String(synchronized)} />;
  },
}));

const releaseMotion = vi.fn(async () => true);
const pause = vi.fn(() => true);
const play = vi.fn(() => true);

const base: MediaVideo = {
  id: "paired", location_path: "C:/media", display_name: "Take 07", size_bytes: 1, modified_at: "2026-09-01T00:00:00Z",
  duration_ms: 60_000, has_funscript: true, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: [],
};

function page(video: MediaVideo, locked = false, stopSequence = 3) {
  return (
    <VideoPlayerPage
      video={video} locked={locked} stopSequence={stopSequence} hostLocked={false} hostAdministration canCurate
      toolsAvailable={false} conversionBusy={false} onBack={vi.fn()} onVideoUpdate={vi.fn()} onRequestConversion={vi.fn()} onEditDetails={vi.fn()}
    />
  );
}
const renderPage = (video: MediaVideo, locked = false) => render(page(video, locked));

const option = (name: string) => screen.getByRole("radio", { name });

describe("motion source beside the video", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    app.modesRunning = false;
    app.mediaID = "";
    releaseMotion.mockResolvedValue(true);
    localStorage.setItem("magichandy-video-chat-open", "true");
    vi.mocked(api.getChatSessions).mockResolvedValue({
      active_session_id: "session-2",
      sessions: [{ id: "session-2", title: "Evening chat", saved: false, active: true, message_count: 1, latest_seq: 1, created_at: "", updated_at: "" }],
    });
    vi.mocked(api.stopMode).mockResolvedValue({});
  });

  it("lets a paired script drive by default and keeps the chat to words", async () => {
    renderPage(base);
    expect(option("Script")).toBeChecked();
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "true");
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "script");
    expect(screen.getByText("The script moves the device. The chat talks but cannot change the motion.")).toBeInTheDocument();
  });

  it("offers no script for a plain video and starts with nothing moving", async () => {
    renderPage({ ...base, id: "plain", has_funscript: false });
    expect(option("Script")).toBeDisabled();
    expect(option("Off")).toBeChecked();
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "false");
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "off");
  });

  it("shows existing background motion as Chat until Off is acknowledged", async () => {
    app.modesRunning = true;
    renderPage({ ...base, id: "plain", has_funscript: false });
    expect(option("Chat")).toBeChecked();
    fireEvent.click(option("Off"));
    await waitFor(() => expect(option("Off")).toBeChecked());
    expect(api.stopMode).toHaveBeenCalledTimes(1);
  });

  it("keeps the new video's script selected while a previous video's run drains", async () => {
    app.mediaID = "previous-video";
    renderPage(base);
    expect(option("Script")).toBeChecked();
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "script");
  });

  it("hands motion to the chat without the script, and stops chat motion when leaving it", async () => {
    renderPage(base);
    fireEvent.click(option("Chat"));
    await waitFor(() => expect(option("Chat")).toBeChecked());
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "false");
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "chat");
    expect(screen.getByText("The chat moves the device. It is not synced to the picture.")).toBeInTheDocument();
    expect(api.stopMode).not.toHaveBeenCalled();

    fireEvent.click(option("Off"));
    await waitFor(() => expect(api.stopMode).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "off");

    fireEvent.click(option("Script"));
    await waitFor(() => expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "true"));
    expect(api.stopMode).toHaveBeenCalledTimes(1);
  });

  it("does not expose a new source until the old script has closed", async () => {
    let finish!: (ok: boolean) => void;
    releaseMotion.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    renderPage(base);
    fireEvent.click(option("Chat"));
    expect(option("Script")).toBeChecked();
    expect(screen.getByRole("group", { name: "Motion source" })).toBeDisabled();
    await act(async () => finish(true));
    expect(option("Chat")).toBeChecked();
    expect(releaseMotion).toHaveBeenCalledWith(3);
    expect(play).not.toHaveBeenCalled();
  });

  it("keeps the old source when stopping motion fails", async () => {
    renderPage(base);
    fireEvent.click(option("Chat"));
    await waitFor(() => expect(option("Chat")).toBeChecked());
    vi.mocked(api.stopMode).mockRejectedValueOnce(new Error("transport did not stop"));
    fireEvent.click(option("Script"));
    await waitFor(() => expect(screen.getByRole("group", { name: "Motion source" })).not.toBeDisabled());
    expect(option("Chat")).toBeChecked();
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "false");
    expect(play).not.toHaveBeenCalled();
  });

  it("does not apply a delayed handoff after Stop", async () => {
    let finish!: (ok: boolean) => void;
    releaseMotion.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    const view = renderPage(base);
    fireEvent.click(option("Chat"));
    view.rerender(page(base, false, 4));
    await act(async () => finish(true));
    expect(option("Script")).toBeChecked();
    expect(play).not.toHaveBeenCalled();
  });

  it("does not switch from a tab that cannot command motion", async () => {
    renderPage(base, true);
    expect(screen.getByRole("group", { name: "Motion source" })).toBeDisabled();
    await screen.findByTestId("chat-panel");
  });
});
