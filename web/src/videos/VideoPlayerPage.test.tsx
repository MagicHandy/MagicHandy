import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { VideoPlayerPage } from "./VideoPlayerPage";

vi.mock("../api/client", () => ({ api: { getChatSessions: vi.fn(), stopMode: vi.fn() } }));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { chat: { active_session_id: "session-2" } } }),
  useToast: () => ({ show: vi.fn() }),
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: ({ motionOwner }: { motionOwner?: string }) => <div data-testid="chat-panel" data-motion-owner={motionOwner ?? "chat"} />,
}));
vi.mock("../components/SyncedVideoPlayer", () => ({
  SyncedVideoPlayer: ({ synchronized }: { synchronized?: boolean }) => <div data-testid="player" data-synchronized={String(synchronized)} />,
}));

const base: MediaVideo = {
  id: "paired", location_path: "C:/media", display_name: "Take 07", size_bytes: 1, modified_at: "2026-09-01T00:00:00Z",
  duration_ms: 60_000, has_funscript: true, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: [],
};

function renderPage(video: MediaVideo, locked = false) {
  return render(
    <VideoPlayerPage
      video={video} locked={locked} stopSequence={3} hostLocked={false} hostAdministration canCurate
      toolsAvailable={false} conversionBusy={false} onBack={vi.fn()} onVideoUpdate={vi.fn()} onRequestConversion={vi.fn()} onEditDetails={vi.fn()}
    />,
  );
}

const option = (name: string) => screen.getByRole("radio", { name });

describe("motion source beside the video", () => {
  beforeEach(() => {
    vi.resetAllMocks();
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

  it("hands motion to the chat without the script, and stops chat motion when leaving it", async () => {
    renderPage(base);
    fireEvent.click(option("Chat"));
    expect(option("Chat")).toBeChecked();
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "false");
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "chat");
    expect(screen.getByText("The chat moves the device. It is not synced to the picture.")).toBeInTheDocument();
    expect(api.stopMode).not.toHaveBeenCalled();

    fireEvent.click(option("Off"));
    await waitFor(() => expect(api.stopMode).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("chat-panel")).toHaveAttribute("data-motion-owner", "off");

    fireEvent.click(option("Script"));
    expect(screen.getByTestId("player")).toHaveAttribute("data-synchronized", "true");
    expect(api.stopMode).toHaveBeenCalledTimes(1);
  });

  it("does not switch from a tab that cannot command motion", () => {
    renderPage(base, true);
    expect(screen.getByRole("group", { name: "Motion source" })).toBeDisabled();
  });
});
