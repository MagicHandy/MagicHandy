import { describe, expect, it } from "vitest";
import { statusSnapshotEqual } from "./statusSnapshotEqual";
import type { StatusSnapshot } from "../api/types";

function snap(overrides: Partial<StatusSnapshot> = {}): StatusSnapshot {
  return {
    intiface_connected: true,
    intiface_url: "",
    intiface_error: null,
    device_label: "dev",
    device_connected: true,
    use_mock: false,
    persona_id: "p1",
    persona_name: "P",
    ollama_model: "",
    ollama_url: "",
    operation_mode: "manual",
    intensity: 50,
    min_position: 0,
    max_position: 100,
    buffer_sec: 0,
    queue_preview: [],
    phase: "idle",
    auto_running: false,
    emergency_stop: false,
    queue_blocks: 0,
    ...overrides,
  };
}

describe("statusSnapshotEqual", () => {
  it("treats identical snapshots as equal", () => {
    const a = snap({ intensity: 60, queue_blocks: 3 });
    const b = snap({ intensity: 60, queue_blocks: 3 });
    expect(statusSnapshotEqual(a, b)).toBe(true);
  });

  it("detects queue_preview length change immediately", () => {
    const a = snap({ queue_preview: [{ block_id: "a", duration_ms: 1, intensity: 1 }] });
    const b = snap({ queue_preview: [] });
    expect(statusSnapshotEqual(a, b)).toBe(false);
  });

  it("detects queue_preview head/tail block_id change without deep compare", () => {
    const a = snap({
      queue_preview: [
        { block_id: "a", duration_ms: 1, intensity: 1 },
        { block_id: "b", duration_ms: 1, intensity: 1 },
      ],
    });
    const b = snap({
      queue_preview: [
        { block_id: "a", duration_ms: 1, intensity: 1 },
        { block_id: "c", duration_ms: 1, intensity: 1 },
      ],
    });
    expect(statusSnapshotEqual(a, b)).toBe(false);
  });

  it("detects volatile scalar changes", () => {
    const a = snap({ manual_queue_playhead_ms: 100 });
    const b = snap({ manual_queue_playhead_ms: 200 });
    expect(statusSnapshotEqual(a, b)).toBe(false);
  });
});
