import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { VideoChatSide } from "./VideoChatSide";
import { VideoPlayerPage } from "./VideoPlayerPage";

const mounts = vi.hoisted(() => ({ player: 0 }));

vi.mock("../api/client", () => ({ api: { getChatSessions: vi.fn() } }));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { chat: { active_session_id: "session-2" } } }),
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: ({ sessionId, personaName }: { sessionId: string; personaName?: string }) => <div data-testid="chat-panel" data-session={sessionId}>{personaName}</div>,
}));
vi.mock("../components/SyncedVideoPlayer", async () => {
  const { useEffect } = await import("react");
  return {
    SyncedVideoPlayer: () => {
      useEffect(() => { mounts.player += 1; }, []);
      return <div data-testid="player" />;
    },
  };
});

const video: MediaVideo = {
  id: "paired", location_path: "C:/media", display_name: "Take 07", size_bytes: 1, modified_at: "2026-09-01T00:00:00Z",
  duration_ms: 60_000, has_funscript: true, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: [],
};

describe("chat beside the video", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
    mounts.player = 0;
    vi.mocked(api.getChatSessions).mockResolvedValue({
      active_session_id: "session-2",
      sessions: [
        { id: "session-1", title: "Older", saved: true, active: false, message_count: 2, latest_seq: 2, created_at: "", updated_at: "" },
        { id: "session-2", title: "Evening chat", saved: false, active: true, persona_name: "Nova", message_count: 4, latest_seq: 9, created_at: "", updated_at: "" },
      ],
    });
  });

  it("shows the active conversation and says who moves the device", async () => {
    render(<VideoChatSide synchronized onClose={vi.fn()} />);
    expect(await screen.findByTestId("chat-panel")).toHaveAttribute("data-session", "session-2");
    expect(screen.getByText("Evening chat")).toBeInTheDocument();
    expect(screen.getByText("The video's script moves the device. If the chat starts motion, it takes over and the video pauses.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Chat" })).toHaveAttribute("href", "#/chat");
  });

  it("opens beside the player without remounting it and remembers the choice", async () => {
    const props = {
      video, locked: false, stopSequence: 3, hostLocked: false, hostAdministration: true, canCurate: true,
      toolsAvailable: false, conversionBusy: false, onBack: vi.fn(), onVideoUpdate: vi.fn(), onRequestConversion: vi.fn(), onEditDetails: vi.fn(),
    };
    const first = render(<VideoPlayerPage {...props} />);
    expect(screen.queryByRole("complementary", { name: "Chat beside the video" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Chat" }));
    expect(await screen.findByRole("complementary", { name: "Chat beside the video" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chat" })).toHaveAttribute("aria-pressed", "true");
    expect(document.querySelector("[data-fullscreen-root]")).toContainElement(screen.getByTestId("player"));
    expect(mounts.player).toBe(1);

    fireEvent.click(screen.getByRole("button", { name: "Close chat" }));
    expect(screen.queryByRole("complementary", { name: "Chat beside the video" })).not.toBeInTheDocument();
    expect(mounts.player).toBe(1);

    fireEvent.click(screen.getByRole("button", { name: "Chat" }));
    first.unmount();
    render(<VideoPlayerPage {...props} />);
    await waitFor(() => expect(screen.getByRole("complementary", { name: "Chat beside the video" })).toBeInTheDocument());
  });
});
