// The phone remote (internal/remote, ADR 0032). A phone queues commands; the
// desktop tab that holds control carries them out with its own controls and
// reports what it shows. None of these reach the device directly.

import type { MotionSource } from "../videos/motionSource";

export interface RemoteVideoPresence {
  video_id: string;
  title: string;
  /** The viewer's intent, as the desktop's play/pause control shows it. */
  playing: boolean;
  position_ms: number;
  duration_ms: number;
  volume: number;
  muted: boolean;
  rate: number;
  synchronized: boolean;
  sync_state?: string;
  /** False while a paired script is still loading. */
  ready: boolean;
  /** What moves the device while this video plays. */
  motion_source?: MotionSource;
  /** A paired script exists, so Script is a possible source. */
  has_script?: boolean;
}

export interface RemoteChatPresence {
  session_id: string;
  persona_name?: string;
  latest_seq?: number;
  revision?: number;
  busy: boolean;
  ready: boolean;
}

export interface RemoteOutcome {
  command_id: string;
  ok: boolean;
  error?: string;
  at?: string;
}

export interface RemotePresence {
  route: string;
  video?: RemoteVideoPresence;
  chat?: RemoteChatPresence;
  outcomes?: RemoteOutcome[];
}

export interface RemoteState {
  stop_sequence?: number;
  can_control?: boolean;
  revision: number;
  connected: boolean;
  /** The desktop is signed in to another account; every detail is withheld. */
  other_account?: boolean;
  route?: string;
  video?: RemoteVideoPresence;
  chat?: RemoteChatPresence;
  pending: number;
  recent: RemoteOutcome[];
  updated_at?: string;
}

export interface RemoteVideoItem { id: string; title: string; duration_ms: number | null; has_funscript: boolean }
export interface RemoteDisplayMessage { seq: number; role: string; content: string; truncated?: boolean }

export type RemoteCommandInput = (
  | { target: "video"; action: "play" | "pause" | "toggle" | "close" }
  | { target: "video"; action: "seek" | "seek_by"; ms: number }
  | { target: "video"; action: "volume" | "rate"; value: number }
  | { target: "video"; action: "mute"; flag: boolean }
  | { target: "video"; action: "open"; video_id: string }
  | { target: "video"; action: "source"; source: MotionSource }
  | { target: "chat"; action: "open" }
  | { target: "chat"; action: "send"; text: string }
) & { video_id?: string; session_id?: string };

export interface RemoteCommand {
  id: string;
  sequence: number;
  target: "video" | "chat";
  action: string;
  video_id?: string;
  session_id?: string;
  stop_sequence: number;
  ms?: number;
  value?: number;
  flag?: boolean;
  text?: string;
  source?: string;
  issued_at: string;
}

export interface RemoteClaim {
  command: RemoteCommand;
  remaining_ms: number;
}
