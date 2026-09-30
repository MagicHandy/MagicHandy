import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RemoteClaim, RemoteCommand, RemotePresence } from "../api/remote-types";
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
    getStopSequence: () => 3,
    releaseMotion: vi.fn(async () => true),
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
const delivered = new Map<string, RemoteCommand>();
function command(target: "video" | "chat", action: string, fields: Partial<RemoteCommand> = {}): RemoteCommand {
  sequence += 1;
  const value = { id: `command-${sequence}`, sequence, target, action, stop_sequence: 3, video_id: "clip", session_id: "session-2", issued_at: "2026-09-27T12:00:00Z", ...fields };
  delivered.set(value.id, value);
  return value;
}

function setup() {
  const reports: RemotePresence[] = [];
  const deps = {
    claim: vi.fn(async (id: string): Promise<RemoteClaim> => ({ command: delivered.get(id)!, remaining_ms: 10_000 })),
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
  executor.setAdmission(3, "lease-1");
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

  it.each(["stop", "local-stop", "lease", "hidden", "expired"])("rejects a delayed claim after %s", async (change) => {
    const { executor, deps } = setup();
    const video = fakeHandle();
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    executor.setEligible(true);
    let finish!: (claim: RemoteClaim) => void;
    deps.claim.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    const intent = command("video", "play");
    const pending = executor.execute(intent);
    await Promise.resolve();
    if (change === "stop") executor.setAdmission(4, "lease-1");
    if (change === "local-stop") executor.cancelPending();
    if (change === "lease") executor.setAdmission(3, "lease-2");
    if (change === "hidden") { executor.setEligible(false); executor.setEligible(true); }
    if (change === "expired") vi.setSystemTime(Date.now() + 11_000);
    finish({ command: intent, remaining_ms: 10_000 });
    await pending;
    expect(video.commands.play).not.toHaveBeenCalled();
    executor.dispose();
  });

  it("never runs a canceled claim or retargets an old command", async () => {
    const { executor, deps } = setup();
    const video = fakeHandle();
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    executor.setEligible(true);
    deps.claim.mockRejectedValueOnce(new Error("cleared by Stop"));
    await executor.execute(command("video", "play"));
    await executor.execute(command("video", "seek", { video_id: "old-video", ms: 1234 }));
    const send = vi.fn(() => true);
    executor.setChat({ sessionId: "new-chat", personaName: "Nova", busy: false, ready: true, send });
    await executor.execute(command("chat", "send", { session_id: "old-chat", text: "hello" }));
    expect(video.commands.play).not.toHaveBeenCalled();
    expect(video.commands.seekTo).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
    executor.dispose();
  });

  it("waits for the source handoff before acknowledging it or running the next command", async () => {
    const { executor, deps, reports } = setup();
    const video = fakeHandle();
    let finish!: (ok: boolean) => void;
    const setMotionSource = vi.fn(() => new Promise<boolean>((resolve) => { finish = resolve; }));
    executor.setVideo({ handle: video.handle, title: "Take 07", hasScript: true, setMotionSource });
    executor.setEligible(true);
    const switching = executor.execute(command("video", "source", { source: "chat" }));
    await vi.advanceTimersByTimeAsync(0);
    const playing = executor.execute(command("video", "play"));
    expect(deps.claim).toHaveBeenCalledTimes(1);
    expect(reports.flatMap((report) => report.outcomes ?? [])).toHaveLength(0);
    finish(false);
    await switching;
    await playing;
    await vi.advanceTimersByTimeAsync(250);
    expect(reports.flatMap((report) => report.outcomes ?? []).map((outcome) => outcome.ok)).toEqual([false, true]);
    executor.dispose();
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
    await executor.execute(seek);
    await executor.execute(seek);
    await executor.execute(command("video", "volume", { value: 0.4 }));
    await executor.execute(command("video", "mute", { flag: true }));
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
    await executor.execute(command("video", "play"));
    const video = fakeHandle();
    vi.mocked(video.commands.play).mockReturnValue(false);
    executor.setVideo({ handle: video.handle, title: "Take 07" });
    await executor.execute(command("video", "play"));
    await executor.execute(command("video", "eject"));
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
    await executor.execute(command("video", "play"));
    await executor.execute(command("video", "toggle"));
    expect(video.commands.play).not.toHaveBeenCalled();
    expect(video.commands.toggle).not.toHaveBeenCalled();

    // Pausing needs no click, and a muted video may always start.
    await executor.execute(command("video", "pause"));
    video.update({ muted: true });
    await executor.execute(command("video", "play"));
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
    const setMotionSource = vi.fn(async () => true);
    executor.setVideo({ handle: video.handle, title: "Take 07", motionSource: "chat", hasScript: true, setMotionSource });
    await executor.execute(command("video", "source", { source: "off" }));
    expect(setMotionSource).toHaveBeenLastCalledWith("off", 3);

    // A switch to the script mid-play starts a new run, which needs sound.
    deps.canPlaySound.mockReturnValue(false);
    await executor.execute(command("video", "source", { source: "script" }));
    expect(setMotionSource).toHaveBeenCalledTimes(1);

    executor.setVideo({ handle: video.handle, title: "Take 07", motionSource: "off", hasScript: false, setMotionSource });
    await executor.execute(command("video", "source", { source: "script" }));
    await executor.execute(command("video", "source", { source: "autopilot" }));
    await vi.advanceTimersByTimeAsync(250);
    expect(reports.flatMap((report) => report.outcomes ?? []).map((outcome) => outcome.error ?? "ok")).toEqual([
      "ok", REMOTE_OUTCOMES.needsClick, REMOTE_OUTCOMES.noScript, REMOTE_OUTCOMES.unknown,
    ]);
    expect(last(reports)?.video).toMatchObject({ motion_source: "off", has_script: false });
  });

  it("opens and closes videos by navigating, and never runs commands while not eligible", async () => {
    const { executor, deps } = setup();
    await executor.execute(command("video", "open", { video_id: "a b" }));
    expect(deps.navigate).not.toHaveBeenCalled();

    executor.setEligible(true);
    await executor.execute(command("video", "open", { video_id: "a b" }));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/videos/a%20b");
    executor.setVideo({ handle: fakeHandle().handle, title: "Take 07" });
    await executor.execute(command("video", "close"));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/videos");
  });

  it("sends chat through the open conversation and refuses while it answers", async () => {
    const { executor, deps, reports } = setup();
    executor.setEligible(true);
    const openChat = vi.fn();
    executor.setVideo({ handle: fakeHandle().handle, title: "Take 07", openChat });
    await executor.execute(command("chat", "open"));
    expect(openChat).toHaveBeenCalledTimes(1);
    executor.setVideo(null);
    await executor.execute(command("chat", "open"));
    expect(deps.navigate).toHaveBeenLastCalledWith("#/chat");

    const send = vi.fn(() => true);
    const chat: RemoteChatSurface = { sessionId: "session-2", personaName: "Nova", busy: false, ready: true, send };
    executor.setChat(chat);
    await executor.execute(command("chat", "send", { text: "  hello  " }));
    expect(send).toHaveBeenCalledWith("hello", 3);
    executor.setChat({ ...chat, busy: true });
    await executor.execute(command("chat", "send", { text: "again" }));
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
    await executor.execute(command("video", "pause"));
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
