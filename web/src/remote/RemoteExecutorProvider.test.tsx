import { render, waitFor } from "@testing-library/react";
import { StrictMode, useMemo } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { REMOTE_COMMAND_EVENT, api } from "../api/client";
import type { VideoPlayerHandle } from "../media/playbackController";
import { RemoteExecutorProvider, useRemoteVideoSurface } from "./RemoteExecutorProvider";

const app = vi.hoisted(() => ({ value: { backendOnline: true, readOnly: false, state: { capabilities: { control: true } } } }));

vi.mock("../state/app-state", () => ({ useAppState: () => app.value }));
vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return { ...actual, api: { reportRemotePresence: vi.fn(), withdrawRemotePresence: vi.fn() } };
});

function player(): VideoPlayerHandle {
  const snapshot = {
    playbackIntent: true, sync: { active: false, state: "idle" as const }, syncError: "", operation: null,
    currentTimeMillis: 1_000, durationMillis: 60_000, volume: 1, muted: false, playbackRate: 1,
    script: null, scriptLoading: false, scriptError: "",
  };
  return {
    videoID: "clip",
    synchronized: false,
    commands: {
      play: vi.fn(() => true), pause: vi.fn(() => true), toggle: vi.fn(() => true), seekTo: vi.fn(() => true),
      seekBy: vi.fn(() => true), setVolume: vi.fn(() => true), setMuted: vi.fn(() => true), setRate: vi.fn(() => true),
    },
    getSnapshot: () => snapshot,
    subscribe: () => () => undefined,
  };
}

function Player({ handle }: { handle: VideoPlayerHandle }) {
  useRemoteVideoSurface(useMemo(() => ({ handle, title: "Take 07" }), [handle]));
  return null;
}

describe("remote executor provider", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    app.value = { backendOnline: true, readOnly: false, state: { capabilities: { control: true } } };
    vi.mocked(api.reportRemotePresence).mockResolvedValue({ remote: { revision: 1, connected: true, pending: 0, recent: [] } });
    vi.mocked(api.withdrawRemotePresence).mockResolvedValue({ status: "withdrawn" });
  });

  it("carries out commands from the motion stream on the registered player and withdraws when control is lost", async () => {
    const handle = player();
    const view = render(<StrictMode><RemoteExecutorProvider route="videos"><Player handle={handle} /></RemoteExecutorProvider></StrictMode>);
    await waitFor(() => expect(api.reportRemotePresence).toHaveBeenCalledWith(expect.objectContaining({
      route: "videos", video: expect.objectContaining({ video_id: "clip", title: "Take 07", playing: true }),
    })));

    window.dispatchEvent(new CustomEvent(REMOTE_COMMAND_EVENT, {
      detail: { id: "phone-1", sequence: 1, target: "video", action: "pause", issued_at: "" },
    }));
    expect(handle.commands.pause).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(api.reportRemotePresence).toHaveBeenLastCalledWith(expect.objectContaining({
      outcomes: [{ command_id: "phone-1", ok: true }],
    })));

    app.value = { ...app.value, readOnly: true };
    view.rerender(<StrictMode><RemoteExecutorProvider route="videos"><Player handle={handle} /></RemoteExecutorProvider></StrictMode>);
    await waitFor(() => expect(api.withdrawRemotePresence).toHaveBeenCalled());
    window.dispatchEvent(new CustomEvent(REMOTE_COMMAND_EVENT, {
      detail: { id: "phone-2", sequence: 2, target: "video", action: "play", issued_at: "" },
    }));
    expect(handle.commands.play).not.toHaveBeenCalled();
  });
});
