import type { StatusSnapshot } from "../api/types";

const VOLATILE_KEYS: (keyof StatusSnapshot)[] = [
  "motion_position_pct",
  "playback_active",
  "direct_control_active",
  "manual_queue_playing",
  "manual_queue_paused",
  "manual_queue_progress_pct",
  "manual_queue_elapsed_sec",
  "manual_queue_playhead_ms",
  "manual_queue_current_segment_index",
  "manual_queue_count",
  "manual_queue_name",
  "phase",
  "phase_elapsed_sec",
  "phase_remaining_sec",
  "phase_progress_pct",
  "phase_clock_running",
  "phase_locked",
  "phase_ready_to_advance",
  "intensity",
  "buffer_remaining_sec",
  "buffer_sec",
  "queue_blocks",
  "planner_refill_busy",
  "planner_busy",
  "planner_busy_source",
  "chat_pending",
  "emergency_stop",
  "auto_running",
  "device_connected",
  "intiface_connected",
  "handy_connected",
  "llm_connected",
  "footer_status",
  "sync_offset_ms",
  "measured_rtt_ms",
  "persona_id",
  "persona_name",
  "operation_mode",
  "active_pose",
  "pose_label",
];

/** Returns true when snapshots are equivalent for UI purposes (skip re-render). */
export function statusSnapshotEqual(a: StatusSnapshot, b: StatusSnapshot): boolean {
  const aqLen = a.queue_preview?.length ?? 0;
  const bqLen = b.queue_preview?.length ?? 0;
  if (aqLen !== bqLen) return false;

  if (aqLen > 0) {
    const aq = a.queue_preview!;
    const bq = b.queue_preview!;
    if (aq[0].block_id !== bq[0].block_id) return false;
    if (aqLen > 1 && aq[aqLen - 1].block_id !== bq[bqLen - 1].block_id) return false;
  }

  for (const key of VOLATILE_KEYS) {
    if (a[key] !== b[key]) return false;
  }

  if (a.playback_current?.block_id !== b.playback_current?.block_id) return false;

  if (!chatAutoEqual(a.chat_auto, b.chat_auto)) return false;

  return true;
}

function chatAutoEqual(
  a: StatusSnapshot["chat_auto"],
  b: StatusSnapshot["chat_auto"],
): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return (
    a.active === b.active &&
    a.stamina === b.stamina &&
    a.humor === b.humor &&
    a.spice_level === b.spice_level &&
    a.mood_progress === b.mood_progress &&
    a.posicao === b.posicao &&
    a.motion?.velocidade === b.motion?.velocidade &&
    a.motion?.intensidade === b.motion?.intensidade &&
    a.motion?.atraso_ms === b.motion?.atraso_ms &&
    a.motion?.regiao === b.motion?.regiao &&
    a.motion?.tipo_batida === b.motion?.tipo_batida &&
    a.llm_busy === b.llm_busy &&
    a.last_reply === b.last_reply &&
    a.error === b.error
  );
}
