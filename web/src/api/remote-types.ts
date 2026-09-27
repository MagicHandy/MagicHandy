// The phone remote (internal/remote, ADR 0032). A phone queues commands; the
// desktop tab that holds control carries them out with its own controls and
// reports what it shows. None of these reach the device directly.

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
}

export interface RemoteChatPresence {
  session_id: string;
  persona_name?: string;
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

export type RemoteCommandInput =
  | { target: "video"; action: "play" | "pause" | "toggle" | "close" }
  | { target: "video"; action: "seek" | "seek_by"; ms: number }
  | { target: "video"; action: "volume" | "rate"; value: number }
  | { target: "video"; action: "mute"; flag: boolean }
  | { target: "video"; action: "open"; video_id: string }
  | { target: "chat"; action: "open" }
  | { target: "chat"; action: "send"; text: string };

export interface RemoteCommand {
  id: string;
  sequence: number;
  target: "video" | "chat";
  action: string;
  video_id?: string;
  ms?: number;
  value?: number;
  flag?: boolean;
  text?: string;
  issued_at: string;
}
