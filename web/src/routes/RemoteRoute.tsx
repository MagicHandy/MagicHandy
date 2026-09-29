import { t, translateKnown } from "../i18n";
import { useEffect, useState } from "react";
import { SegmentedChoice } from "../components/SetpointControls";
import { RemoteChat } from "../remote/RemoteChat";
import { RemoteVideoControls } from "../remote/RemoteVideoControls";
import { RemoteVideoPicker } from "../remote/RemoteVideoPicker";
import { useRemoteCommands } from "../remote/useRemoteCommands";
import { remotePosition, type RemoteView } from "../remote/useRemoteState";
import "../styles/remote.css";

type RemoteMode = "video" | "chat";
const MODE_KEY = "magichandy-remote-mode";

// A phone remote for the desktop that holds control (ADR 0032): the video
// there, or its chat. Commands go through the desktop's own controls, so the
// persistent Stop below still stops everything, from here or the desktop.
export function RemoteRoute({ remote, canControl, onRefresh }: { remote: RemoteView; canControl: boolean; onRefresh?: () => void }) {
  const { send, status } = useRemoteCommands(remote.state, remote.state?.stop_sequence);
  const [mode, setMode] = useState<RemoteMode>(readMode);
  const state = remote.state;
  const reachable = Boolean(canControl && !remote.stale && state?.connected && !state.other_account);
  const now = useNow(Boolean(state?.video?.playing), 500);

  function chooseMode(next: RemoteMode) {
    setMode(next);
    try {
      localStorage.setItem(MODE_KEY, next);
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }

  return <div className="remote-page">
    <div className="remote-connection-row"><RemoteConnection remote={remote} /><button className="btn btn-secondary small" type="button" onClick={onRefresh || remote.refresh}>{t("Refresh")}</button></div>
    {!canControl ? <p className="remote-notice" role="status">{t("Ask the administrator for a control permission to use this remote.")}</p> : <>
      <SegmentedChoice className="remote-mode" label={t("Control")} value={mode}
        options={[{ value: "video", label: t("Video") }, { value: "chat", label: t("Chat") }]} onChange={chooseMode} />
      {status && <p className="remote-command-status" data-tone={status.tone} role={status.tone === "error" ? "alert" : "status"}>{status.message}</p>}
      {reachable && state && <div className="remote-workspace" data-mode={mode}>
        <section className="remote-video-column" aria-label={t("Video")}>
          <h2 className="remote-pane-title">{t("Video")}</h2>
          {state.video ? <RemoteVideoControls key={`${state.video.video_id}:${state.stop_sequence}`} video={state.video} position={remotePosition(state, remote.receivedAt, now)} send={send} />
            : <p className="remote-notice">{t("The desktop is not showing a video. Open one below.")}</p>}
          <RemoteVideoPicker currentID={state.video?.video_id} send={send} />
        </section>
        <section className="remote-chat-column" aria-label={t("Desktop chat")}>
          <h2 className="remote-pane-title">{t("Desktop chat")}</h2>
          <RemoteChat key={state.chat?.session_id} state={state} send={send} />
        </section>
      </div>}
    </>}
  </div>;
}

function RemoteConnection({ remote }: { remote: RemoteView }) {
  const state = remote.state;
  let tone: "ok" | "warn" | "idle" = "idle";
  let text: string;
  if (!state) {
    text = remote.error ? translateKnown(remote.error) : t("Looking for the desktop…");
  } else if (state.other_account) {
    tone = "warn";
    text = t("The desktop is signed in to another account.");
  } else if (!state.connected) {
    text = t("No desktop is taking commands. On the desktop, take control in MagicHandy and keep that tab in front.");
  } else if (remote.stale) {
    tone = "warn";
    text = t("Reconnecting to the desktop…");
  } else {
    tone = "ok";
    text = t("Connected to the desktop: {page}", { page: pageLabel(state.route) });
  }
  return (
    <p className="remote-connection" data-tone={tone} role="status">
      <span className="remote-dot" aria-hidden="true" />
      {text}
    </p>
  );
}

function pageLabel(route: string | undefined): string {
  switch (route) {
    case "videos": return t("Videos");
    case "chat": return t("Chat");
    case "personas": return t("Personas");
    case "modes": return t("Preset modes");
    case "library": return t("Pattern library");
    case "settings": return t("Settings");
    case "remote": return t("Remote");
    default: return t("Another page");
  }
}

function readMode(): RemoteMode {
  try {
    return localStorage.getItem(MODE_KEY) === "chat" ? "chat" : "video";
  } catch {
    return "video";
  }
}

/** A clock that ticks only while something on screen moves with it. */
function useNow(active: boolean, intervalMillis: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), intervalMillis);
    return () => clearInterval(timer);
  }, [active, intervalMillis]);
  return now;
}
