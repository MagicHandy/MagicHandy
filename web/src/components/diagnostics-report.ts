import type { AppState, MediaToolStatus } from "../api/types";

/**
 * Builds the diagnostics report as plain sectioned text.
 *
 * Text rather than a rendered grid, because the thing people actually do with
 * diagnostics is paste them somewhere. A tile per fact is unreadable at a
 * glance and unpasteable at any length, and a hidden JSON blob behind a Copy
 * button means nobody can see what they are about to send. What is shown is
 * exactly what is copied.
 *
 * Nothing secret can reach here: the report is built from the app state's
 * public projection, in which the Handy connection key and the ElevenLabs key
 * already exist only as `*_key_set` booleans.
 */

interface ReportSection {
  title: string;
  lines: string[];
}

const UNKNOWN = "unknown";

function text(value: unknown, fallback = UNKNOWN): string {
  if (value === null || value === undefined) return fallback;
  const rendered = String(value).trim();
  return rendered === "" ? fallback : rendered;
}

function yesNo(value: unknown): string {
  if (value === undefined || value === null) return UNKNOWN;
  return value ? "yes" : "no";
}

function duration(seconds: unknown): string {
  const total = Number(seconds);
  if (!Number.isFinite(total) || total < 0) return UNKNOWN;
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${Math.floor(total % 60)}s`;
  return `${Math.floor(total)}s`;
}

export function engineStateLabel(state: AppState | null | undefined): string {
  const engine = state?.motion?.engine;
  if (!engine) return "unavailable";
  if (engine.starting) return "starting";
  if (engine.completing) return "stopping";
  if (engine.running) return "running";
  if (engine.paused) return "paused";
  return "idle";
}

/**
 * Reports which video codecs this browser will actually decode.
 *
 * Worth its own section: "my video will not play" is one of the few problems a
 * user cannot diagnose from the app's own state, because the answer lives in
 * the browser rather than in MagicHandy. An H.265 file in a perfectly ordinary
 * .mp4 plays in one browser and not another, and this is the line in the report
 * that says which.
 */
export function browserCodecSupport(): string[] {
  const probes: Array<[string, string]> = [
    ["H.264", 'video/mp4; codecs="avc1.42E01E"'],
    ["H.265 / HEVC", 'video/mp4; codecs="hvc1.1.6.L93.B0"'],
    ["AV1", 'video/mp4; codecs="av01.0.05M.08"'],
    ["VP9", 'video/webm; codecs="vp9"'],
    ["Matroska container", "video/x-matroska"],
  ];
  let element: HTMLVideoElement | null = null;
  try {
    element = document.createElement("video");
  } catch {
    return [`- Codec support: ${UNKNOWN}`];
  }
  return probes.map(([label, type]) => {
    let answer = UNKNOWN;
    try {
      answer = element.canPlayType(type) || "no";
    } catch {
      answer = UNKNOWN;
    }
    return `- ${label}: ${answer}`;
  });
}

function applicationSection(state: AppState | null | undefined): ReportSection {
  return {
    title: "Application",
    lines: [
      `- Version: ${text(state?.version, "dev")}`,
      `- Commit: ${text(state?.commit)}`,
      `- Uptime: ${duration(state?.uptime_seconds)}`,
      `- Data directory: ${text(state?.data_dir, "—")}`,
      `- Datastore: ${text(state?.datastore_path, "—")}`,
    ],
  };
}

function motionSection(state: AppState | null | undefined): ReportSection {
  const engine = state?.motion?.engine;
  const position = engine?.last_sample
    ? `${Math.round(engine.last_sample.position_percent)}%`
    : "—";
  return {
    title: "Motion engine",
    lines: [
      `- Available: ${yesNo(state?.motion?.available)}`,
      `- State: ${engineStateLabel(state)}`,
      `- Estimated position: ${position}`,
      `- Stop sequence: ${text(state?.stop_sequence, "0")}`,
      `- Active mode: ${text(state?.modes?.active_mode, "none")}`,
      ...(state?.motion?.error ? [`- Error: ${state.motion.error}`] : []),
    ],
  };
}

function transportSection(state: AppState | null | undefined): ReportSection {
  const settings = state?.settings;
  const lines = [
    `- Dispatch owner: ${text(settings?.device?.hsp_dispatch_owner)}`,
    `- Connection key set: ${yesNo(settings?.device?.connection_key_set)}`,
    `- Firmware requirement: ${text(settings?.device?.firmware_api_requirement)}`,
  ];
  for (const [label, snapshot] of [
    ["Cloud REST", state?.cloud_transport],
    ["Browser Bluetooth", state?.bluetooth_transport],
  ] as const) {
    if (!snapshot) continue;
    lines.push(
      `- ${label}: connected ${yesNo(snapshot.connected)}, ${text(snapshot.playback_state, "idle")}` +
        (snapshot.last_latency_ms === undefined ? "" : `, ${snapshot.last_latency_ms}ms last`),
    );
    if (snapshot.last_error) lines.push(`  - Last error: ${snapshot.last_error}`);
  }
  const intiface = state?.intiface_transport?.status;
  if (intiface) {
    lines.push(
      `- Intiface: ${text(intiface.playback_state, "idle")}, ${intiface.queue_depth} queued / ${intiface.queue_coverage_ms ?? 0}ms`,
      `- Intiface pending ACKs: ${intiface.pending_acks ?? 0}`,
      `- Intiface ACK latency: ${intiface.last_ack_latency_ms ?? 0}ms last / ${intiface.max_ack_latency_ms ?? 0}ms max`,
      `- Intiface send lateness: ${intiface.last_send_lateness_ms ?? 0}ms last / ${intiface.max_send_lateness_ms ?? 0}ms max`,
    );
  }
  return { title: "Device transport", lines };
}

function modelSection(state: AppState | null | undefined): ReportSection {
  const llm = state?.settings?.llm;
  return {
    title: "Language model",
    lines: [
      `- Provider: ${text(llm?.provider)}`,
      `- Model: ${text(llm?.model)}`,
      `- Prompt set: ${text(llm?.prompt_set)}`,
      `- Reasoning: ${text(llm?.reasoning_mode)}`,
      `- Max output tokens: ${text(llm?.max_output_tokens)}`,
    ],
  };
}

function voiceSection(state: AppState | null | undefined): ReportSection {
  const voice = state?.settings?.voice;
  const lines = [
    `- Enabled: ${yesNo(state?.voice?.enabled)}`,
    `- Speech output: ${text(voice?.tts_provider, "none")}`,
    `- Speech input: ${text(voice?.asr_provider, "none")}`,
    `- ElevenLabs key set: ${yesNo(voice?.elevenlabs_key_set)}`,
  ];
  for (const [name, worker] of Object.entries(state?.voice?.workers ?? {})) {
    lines.push(`- Worker ${name}: ${text(worker?.state)}${worker?.last_error ? ` (${worker.last_error})` : ""}`);
  }
  return { title: "Voice", lines };
}

function mediaSection(
  state: AppState | null | undefined,
  tools: MediaToolStatus | null | undefined,
): ReportSection {
  const media = state?.media;
  let ffmpeg = "not configured";
  if (tools?.available) ffmpeg = `ready (${text(tools.version)})`;
  else if (tools?.configured) ffmpeg = `configured but unusable: ${text(tools.error)}`;
  return {
    title: "Media library",
    lines: [
      `- Catalog available: ${yesNo(media?.available)}`,
      `- Videos: ${text(media?.video_count, "0")}`,
      `- Paired scripts: ${text(media?.paired_count, "0")}`,
      `- FFmpeg: ${ffmpeg}`,
    ],
  };
}

function browserSection(): ReportSection {
  return {
    title: "Browser",
    lines: [
      `- User agent: ${text(typeof navigator === "undefined" ? null : navigator.userAgent)}`,
      ...browserCodecSupport(),
    ],
  };
}

export interface ReportInputs {
  state: AppState | null | undefined;
  tools?: MediaToolStatus | null;
  generatedAt?: Date;
}

/** buildDiagnosticsReport renders the whole report as copyable plain text. */
export function buildDiagnosticsReport({ state, tools, generatedAt }: ReportInputs): string {
  const sections: ReportSection[] = [
    applicationSection(state),
    motionSection(state),
    transportSection(state),
    modelSection(state),
    voiceSection(state),
    mediaSection(state, tools),
    browserSection(),
  ];
  const header = [
    "MagicHandy diagnostics",
    `Generated: ${(generatedAt ?? new Date()).toISOString()}`,
  ];
  const body = sections.flatMap((section) => ["", section.title, ...section.lines]);
  return [...header, ...body].join("\n");
}
