import type { BluetoothPlaybackState } from "../api/types";

// Forward a bounded telemetry view, never arbitrary notification fields or
// data from a stream that this browser has already stopped/replaced.
export function bluetoothPlaybackFeedback(raw: unknown, streamID: number | null, sequence: number, commandID: string | null): BluetoothPlaybackState | undefined {
  if (!raw || typeof raw !== "object" || streamID === null || !commandID) return;
  const state = raw as Record<string, unknown>;
  if (state.stream_id !== streamID || !["not_initialized", "playing", "stopped", "paused", "starving"].includes(String(state.play_state))) return;
  const integer = (key: string, fallback: number, maximum: number) => {
    const value = state[key];
    return typeof value === "number" && Number.isSafeInteger(value) && value >= fallback && value <= maximum ? value : fallback;
  };
  return {
    sequence, command_id: commandID, stream_id: streamID, play_state: String(state.play_state),
    points: integer("points", 0, 1_000_000),
    current_point: integer("current_point", -1, 1_000_000),
    current_time_ms: integer("current_time_ms", -1, 2_147_483_647),
    tail_point_stream_index: integer("tail_point_stream_index", -1, 2_147_483_647),
  };
}
