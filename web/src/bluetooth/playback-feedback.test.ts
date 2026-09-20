import { describe, expect, it } from "vitest";
import { bluetoothPlaybackFeedback } from "./playback-feedback";

describe("Bluetooth playback feedback", () => {
  it("forwards bounded device telemetry for the active stream only", () => {
    const state = { stream_id: 7, play_state: "starving", points: 3, current_point: 9, current_time_ms: 1300, tail_point_stream_index: 15, arbitrary: "private" };
    expect(bluetoothPlaybackFeedback(state, 7, 4, "play-1")).toEqual({ sequence: 4, command_id: "play-1", stream_id: 7, play_state: "starving", points: 3, current_point: 9, current_time_ms: 1300, tail_point_stream_index: 15 });
    expect(bluetoothPlaybackFeedback(state, 8, 5, "play-1")).toBeUndefined();
    expect(bluetoothPlaybackFeedback(state, null, 5, "play-1")).toBeUndefined();
    expect(bluetoothPlaybackFeedback({ ...state, play_state: "anything" }, 7, 5, "play-1")).toBeUndefined();
  });

  it("does not forward invalid numeric fields", () => {
    expect(bluetoothPlaybackFeedback({ stream_id: 2, play_state: "playing", points: Infinity, current_time_ms: -200, current_point: 3.2 }, 2, 1, "add-2")).toMatchObject({ points: 0, current_time_ms: -1, current_point: -1 });
  });
});
