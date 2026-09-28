import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RemoteCommand, RemotePresence } from "../api/remote-types";
import type { PlaybackSnapshot, VideoPlayerCommands, VideoPlayerHandle } from "../media/playbackController";
import { REMOTE_OUTCOMES, RemoteExecutor, type RemoteChatSurface } from "./executor";

function fakeHandle(overrides: Partial<PlaybackSnapshot> = {}, synchronized = true) {
  let snapshot: PlaybackSnapshot = {
    playbackIntent: false,
    sync: { active: false, state: "idle" },
    syncError: "",
    operation: null,
    currentTimeMillis: 12_000,
    durationMillis: 90_000,
    volume: 0.8,
    muted: false,
    playbackRate: 1,
    script: null,
    scriptLoading: false,
    scriptError: "",
    ...overrides,
  };
  const listeners = new Set<() => void>();
  const commands: VideoPlayerCommands = {
    play: vi.fn(() => true),
    pause: vi.fn(() => true),
    toggle: vi.fn(() => true),
    seekTo: vi.fn(() => true),
    seekBy: vi.fn(() => true),
    setVolume: vi.fn(() => true),
    setMuted: vi.fn(() => true),
    setRate: vi.fn(() => true),
  };
  const handle: VideoPlayerHandle = {
    videoID: "clip",
    synchronized,
    commands,
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
  return {
    handle,
    commands,
    update(patch: Partial<PlaybackSnapshot>) {
      snapshot = { ...snapshot, ...patch };
      for (const listener of listeners) listener();
    },
  };
}

const last = <T,>(items: T[]): T | undefined => items[items.length - 1];

let sequence = 0;
function command(target: "video" | "chat", action: string, fields: Partial<RemoteCommand> = {}): RemoteCommand {
  sequence += 1;
  return { id: `command-${sequence}`, sequence, target, action, issued_at: "2026-09-27T12:00:00Z", ...fields };
}

function setup() {
  const reports: RemotePresence[] = [];
  const deps = {
    report: vi.fn(async (presence: RemotePresence) => {
      reports.push(presence);
      return {};
    }),
    withdraw: vi.fn(),
    navigate: vi.fn(),
    canPlaySound: vi.fn(() => true),
    now: () => Date.now(),
  };
  const executor = new RemoteExecutor(deps);
  return { executor, deps, reports };
}

describe("remote executor", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-27T12:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports what the tab shows only while it is eligible", async () => {
    const { executor, deps, reports } = setup();
    const video = fakeHandle({ playbackIntent: true });
    executor.setRoute("videos");
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    await vi.advanceTimersByTimeAsync(1_000);
    expect(deps.report).not.toHaveBeenCalled();

    executor.setEligible(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(last(reports)).toEqual({
      route: "videos",
      video: {
        video_id: "clip", title: "Take 07", playing: true, position_ms: 12_000, duration_ms: 90_000,
        volume: 0.8, muted: false, rate: 1, synchronized: true, sync_state: "idle", ready: true, has_script: false,
      },
    });

    // A keepalive keeps the phone's view alive without any change.
    await vi.advanceTimersByTimeAsync(5_000);
    expect(deps.report).toHaveBeenCalledTimes(2);

    executor.setEligible(false);
    expect(deps.withdraw).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(deps.report).toHaveBeenCalledTimes(2);
  });

  it("runs video commands through the player's own commands and reports each outcome once", async () => {
    const { executor, reports } = setup();
    const video = fakeHandle();
    executor.setEligible(true);
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    await vi.advanceTimersByTimeAsync(0);

    const seek = command("video", "seek", { ms: 30_000 });
    executor.execute(seek);
    executor.execute(seek);
    executor.execute(command("video", "volume", { value: 0.4 }));
    executor.execute(command("video", "mute", { flag: true }));
    await vi.advanceTimersByTimeAsync(250);

    expect(video.commands.seekTo).toHaveBeenCalledTimes(1);
    expect(video.commands.seekTo).toHaveBeenCalledWith(30_000);
    expect(video.commands.setVolume).toHaveBeenCalledWith(0.4);
    expect(video.commands.setMuted).toHaveBeenCalledWith(true);
    const outcomes = reports.flatMap((report) => report.outcomes ?? []);
    expect(outcomes.map((outcome) => outcome.ok)).toEqual([true, true, true]);
    expect(outcomes[0].command_id).toBe(seek.id);
  });

  it("says why a command could not run", async () => {
    const { executor, reports } = setup();
    executor.setEligible(true);
    executor.execute(command("video", "play"));
    const video = fakeHandle();
    vi.mocked(video.commands.play).mockReturnValue(false);
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    executor.execute(command("video", "play"));
    executor.execute(command("video", "eject"));
    await vi.advanceTimersByTimeAsync(250);
    expect(reports.flatMap((report) => report.outcomes ?? []).map((outcome) => outcome.error)).toEqual([
      REMOTE_OUTCOMES.noVideo, REMOTE_OUTCOMES.notReady, REMOTE_OUTCOMES.unknown,
    ]);
  });

  it("asks for a click on the desktop instead of starting sound the browser would refuse", async () => {
    const { executor, deps, reports } = setup();
    deps.canPlaySound.mockReturnValue(false);
    executor.setEligible(true);
    const video = fakeHandle();
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    executor.execute(command("video", "play"));
    executor.execute(command("video", "toggle"));
    expect(video.commands.play).not.toHaveBeenCalled();
    expect(video.commands.toggle).not.toHaveBeenCalled();

    // Pausing needs no click, and a muted video may always start.
    executor.execute(command("video", "pause"));
    video.update({ muted: true });
    executor.execute(command("video", "play"));
    expect(video.commands.pause).toHaveBeenCalledTimes(1);
    expect(video.commands.play).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(250);
    expect(reports.flatMap((report) => report.outcomes ?? []).map((outcome) => outcome.error ?? "ok")).toEqual([
      REMOTE_OUTCOMES.needsClick, REMOTE_OUTCOMES.needsClick, "ok", "ok",
    ]);
  });

  it("switches the motion source through the page and never restarts a script the browser would refuse", async () => {
    const { executor, deps, reports } = setup();
    executor.setEligible(true);
    const video = fakeHandle({ playbackIntent: true });
    const setMotionSource = vi.fn(() => true);
    executor.setVideo({ handle: video.handle, title: "Take 07", motionSource: "chat", hasScript: true, setMotionSource });
    executor.execute(command("video", "source", { source: "off" }));
    expect(setMotionSource).toHaveBeenLastCalledWith("off");

    // A switch to the script mid-play starts a new run, which needs sound.
    deps.canPlaySound.mockReturnValue(false);
    executor.execute(command("video", "source", { source: "script" }));
    expect(setMotionSource).toHaveBeenCalledTimes(1);

    executor.setVideo({ handle: video.handle, title: "Take 07", motionSource: "off", hasScript: false, setMotionSource });
    executor.execute(command("video", "source", { source: "script" }));
    executor.execute(command("video", "source", { source: "autopilot" }));
    await vi.advanceTimersByTimeAsync(250);
    expect(reports.flatMap((report) => report.outcomes ?? []).map((outcome) => outcome.error ?? "ok")).toEqual([
      "ok", REMOTE_OUTCOMES.needsClick, REMOTE_OUTCOMES.noScript, REMOTE_OUTCOMES.unknown,
    ]);
    expect(last(reports)?.video).toMatchObject({ motion_source: "off", has_script: false });
  });

  it("opens and closes videos by navigating, and never runs commands while not eligible", async () => {
    const { executor, deps } = setup();
    executor.execute(command("video", "open", { video_id: "a b" }));
    expect(deps.navigate).not.toHaveBeenCalled();

    executor.setEligible(true);
    executor.execute(command("video", "open", { video_id: "a b" }));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/videos/a%20b");
    executor.setVideo({ handle: fakeHandle().handle, title: "Take 07" });
    executor.execute(command("video", "close"));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/videos");
  });

  it("sends chat through the open conversation and refuses while it answers", async () => {
    const { executor, deps, reports } = setup();
    executor.setEligible(true);
    const openChat = vi.fn();
    executor.setVideo({ handle: fakeHandle().handle, title: "Take 07", openChat });
    executor.execute(command("chat", "open"));
    expect(openChat).toHaveBeenCalledTimes(1);
    executor.setVideo(null);
    executor.execute(command("chat", "open"));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/chat");

    const send = vi.fn();
    const chat: RemoteChatSurface = { sessionId: "session-2", personaName: "Nova", busy: false, ready: true, send };
    executor.setChat(chat);
    executor.execute(command("chat", "send", { text: "  hello  " }));
    expect(send).toHaveBeenCalledWith("hello");
    executor.setChat({ ...chat, busy: true });
    executor.execute(command("chat", "send", { text: "again" }));
    expect(send).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(250);

    expect(last(reports)?.chat).toEqual({ session_id: "session-2", persona_name: "Nova", busy: true, ready: true });
    expect(last(reports.flatMap((report) => report.outcomes ?? []))?.error).toBe(REMOTE_OUTCOMES.chatBusy);
  });

  it("reports player changes promptly but not every position tick", async () => {
    const { executor, deps } = setup();
    const video = fakeHandle({ playbackIntent: true });
    executor.setEligible(true);
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.report).toHaveBeenCalledTimes(1);

    // Playback advancing as expected is not news.
    await vi.advanceTimersByTimeAsync(1_000);
    video.update({ currentTimeMillis: 13_000 });
    await vi.advanceTimersByTimeAsync(250);
    expect(deps.report).toHaveBeenCalledTimes(1);

    // A seek or a pause is.
    video.update({ currentTimeMillis: 60_000 });
    await vi.advanceTimersByTimeAsync(250);
    expect(deps.report).toHaveBeenCalledTimes(2);
    video.update({ playbackIntent: false });
    await vi.advanceTimersByTimeAsync(250);
    expect(deps.report).toHaveBeenCalledTimes(3);
  });

  it("keeps outcomes for the next report and backs off while reports are refused", async () => {
    const { executor, deps, reports } = setup();
    deps.report
      .mockRejectedValueOnce(new Error("this tab does not hold control"))
      .mockRejectedValueOnce(new Error("this tab does not hold control"))
      .mockRejectedValueOnce(new Error("this tab does not hold control"));
    executor.setEligible(true);
    executor.setVideo({ handle: fakeHandle().handle, title: "Take 07" });
    executor.execute(command("video", "pause"));
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.report).toHaveBeenCalledTimes(1);
    // Retries wait 0.5 s, then 1 s, then 2 s; not one every 200 ms.
    await vi.advanceTimersByTimeAsync(499);
    expect(deps.report).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(deps.report).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(deps.report).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(deps.report).toHaveBeenCalledTimes(4);
    expect(reports).toHaveLength(1);
    expect(last(reports)?.outcomes?.map((outcome) => outcome.ok)).toEqual([true]);
  });

  it("withdraws a report that lands after the tab stopped being eligible", async () => {
    let finish: () => void = () => undefined;
    const { executor, deps } = setup();
    deps.report.mockImplementationOnce(() => new Promise<object>((resolve) => {
      finish = () => resolve({});
    }));
    executor.setEligible(true);
    await vi.advanceTimersByTimeAsync(0);
    executor.setEligible(false);
    expect(deps.withdraw).not.toHaveBeenCalled();
    finish();
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.withdraw).toHaveBeenCalledTimes(1);
  });
});
