import { describe, expect, it } from "vitest";
import type { VoiceWorkerStatus } from "../api/types";
import { splitWorkerError, voicePhaseTone, voiceWorkerPhase, workerSignature, type VoicePhaseInput } from "./voice-worker-phase";

function worker(overrides: Partial<VoiceWorkerStatus>): VoiceWorkerStatus {
  return { role: "asr", state: "running", configured: true, worker_queue_depth: 0, queue_depth: 0, ...overrides };
}

const phase = (input: Partial<VoicePhaseInput>) => voiceWorkerPhase({ loaded: true, ...input });

describe("voiceWorkerPhase", () => {
  it("distinguishes checking, unavailable, off, unsaved and needs-setup", () => {
    expect(phase({ loaded: false })).toBe("checking");
    expect(phase({ loaded: false, unavailable: true })).toBe("unavailable");
    expect(phase({ worker: worker({ state: "disabled" }) })).toBe("off");
    expect(phase({ worker: worker({ state: "disabled" }), dirty: true })).toBe("unsaved");
    expect(phase({ worker: worker({ state: "not_configured" }) })).toBe("setup");
    expect(phase({ worker: worker({ state: "not_configured" }), dirty: true })).toBe("unsaved");
  });

  it("shows a missing module before unsaved edits so repair stays reachable", () => {
    expect(phase({ worker: worker({ state: "not_configured" }), moduleBlocked: true, dirty: true })).toBe("setup");
    expect(phase({ moduleBlocked: true, repairFailed: true })).toBe("setup_failed");
    expect(phase({ moduleBlocked: true, repairActive: true })).toBe("installing");
  });

  it("only calls a worker ready when its model is ready and nothing failed", () => {
    expect(phase({ worker: worker({ model_state: "ready" }) })).toBe("ready");
    expect(phase({ worker: worker({ model_state: "ready", last_error: "managed TTS server exited unexpectedly" }) })).toBe("failed");
    expect(phase({ worker: worker({ model_state: "unloaded" }) })).toBe("unloaded");
    expect(phase({ worker: worker({ model_state: "unloading" }) })).toBe("stopping");
    expect(phase({ worker: worker({ model_state: "" }) })).toBe("busy");
    expect(phase({ worker: worker({ model_state: "unknown" }) })).toBe("busy");
  });

  it("shows loading only while a running worker's load is tracked without an error", () => {
    expect(phase({ worker: worker({ model_state: "loading" }) })).toBe("loading");
    expect(phase({ worker: worker({ state: "starting" }) })).toBe("starting");
    // The supervisor clears the error when it starts tracking a load, so an
    // error beside "loading" is not a tracked load; it stays a failure.
    expect(phase({ worker: worker({ model_state: "loading", last_error: "context deadline exceeded" }) })).toBe("failed");
    expect(phase({ worker: worker({ state: "crashed", model_state: "loading", last_error: "exit status 1" }) })).toBe("failed");
  });

  it("never decides a failure from the wording of the error", () => {
    expect(phase({ worker: worker({ last_error: "context deadline exceeded" }) })).toBe("failed");
    expect(phase({ worker: worker({ last_error: "context canceled" }) })).toBe("failed");
    expect(phase({ worker: worker({ state: "stopped", last_error: "context deadline exceeded" }) })).toBe("failed");
    expect(phase({ worker: worker({ state: "stopped", last_error: "TTS server did not become ready: port in use" }) })).toBe("failed");
    expect(phase({ worker: worker({ state: "crashed" }) })).toBe("failed");
    expect(phase({ worker: worker({ state: "stopped" }) })).toBe("stopped");
  });

  it("keeps this tab's start pending only until a newer snapshot reports a failure", () => {
    const crashed = worker({ state: "crashed", last_error: "exit status 1" });
    expect(phase({ worker: worker({ state: "stopped" }), pending: "start" })).toBe("starting");
    expect(phase({ worker: crashed, pending: "restart" })).toBe("starting");
    expect(phase({ worker: worker({ model_state: "" }), pending: "load" })).toBe("loading");
    expect(phase({ worker: crashed, pending: "restart", pendingFailed: true })).toBe("failed");
    expect(phase({ worker: worker({ model_state: "ready" }), pending: "start" })).toBe("ready");
    expect(phase({ worker: worker({ model_state: "ready" }), pending: "stop" })).toBe("stopping");
  });

  it("maps phases to the shared status tones", () => {
    expect(voicePhaseTone("ready")).toBe("ok");
    expect(voicePhaseTone("loading")).toBe("pending");
    expect(voicePhaseTone("busy")).toBe("pending");
    expect(voicePhaseTone("failed")).toBe("error");
    expect(voicePhaseTone("setup")).toBe("setup");
    expect(voicePhaseTone("stopped")).toBe("idle");
  });
});

describe("worker helpers", () => {
  it("separates crash output from the cause", () => {
    expect(splitWorkerError(worker({ last_error: "exited — stderr: boom\n", stderr_tail: "boom\n" }))).toEqual({ message: "exited", output: "boom" });
    expect(splitWorkerError(worker({ last_error: "plain failure" }))).toEqual({ message: "plain failure", output: "" });
  });

  it("changes signature when the process, state or error changes", () => {
    const base = worker({ state: "crashed", last_error: "exit status 1" });
    expect(workerSignature(base)).toBe(workerSignature({ ...base, queue_depth: 3 }));
    expect(workerSignature(base)).not.toBe(workerSignature({ ...base, last_error: "exit status 2" }));
    expect(workerSignature(base)).not.toBe(workerSignature({ ...base, state: "running", started_at: "2026-10-26T10:00:00Z" }));
  });
});
