import { t, translateKnown } from "../i18n";
import { useEffect, useState } from "react";
import { SegmentedChoice } from "../components/SetpointControls";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { RemoteChat } from "../remote/RemoteChat";
import { RemoteVideoControls } from "../remote/RemoteVideoControls";
import { RemoteVideoPicker } from "../remote/RemoteVideoPicker";
import { useRemoteCommands } from "../remote/useRemoteCommands";
import { remotePosition, useRemoteState, type RemoteView } from "../remote/useRemoteState";
import { useAppState } from "../state/app-state";
import "../styles/remote.css";

type RemoteMode = "video" | "chat";
const MODE_KEY = "magichandy-remote-mode";

// A phone remote for the desktop that holds control (ADR 0032): the video
// there, or its chat. Commands go through the desktop's own controls, so the
// persistent Stop below still stops everything, from here or the desktop.
export function RemoteRoute() {
  const { backendOnline, readOnly, state: appState } = useAppState();
  const canControl = appState?.capabilities?.control !== false;
  const remote = useRemoteState(backendOnline && canControl);
  const { send, status } = useRemoteCommands(remote.state);
  const [mode, setMode] = useState<RemoteMode>(readMode);
  const state = remote.state;
  const reachable = Boolean(state?.connected && !state.other_account);
  const now = useNow(Boolean(state?.video?.playing), 500);

  function chooseMode(next: RemoteMode) {
    setMode(next);
    try {
      localStorage.setItem(MODE_KEY, next);
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }

  return (
    <>
      <WorkspaceHead title={t("Remote")} lede={t("Control the video or chat on the desktop that holds control.")} />
      <div className="remote-page" data-requires-backend>
        {!canControl ? (
          <p className="remote-notice" role="status">{t("This account can watch but not control. Ask the administrator for a control permission to use the remote.")}</p>
        ) : (
          <>
            <RemoteConnection remote={remote} />
            {!readOnly && state?.route === "remote" && (
              <p className="remote-notice">{t("This tab holds control, so it is the desktop. Use the remote from your phone or another device.")}</p>
            )}
            <SegmentedChoice
              className="remote-mode"
              label={t("Control")}
              value={mode}
              options={[{ value: "video", label: t("Video") }, { value: "chat", label: t("Chat") }]}
              onChange={chooseMode}
            />
            {status && (
              <p className="remote-command-status" data-tone={status.tone} role={status.tone === "error" ? "alert" : "status"}>{status.message}</p>
            )}
            {reachable && state && mode === "video" && (
              <>
                {state.video ? (
                  <RemoteVideoControls video={state.video} position={remotePosition(state, remote.receivedAt, now)} send={send} />
                ) : (
                  <p className="remote-notice">{t("The desktop is not showing a video. Open one below.")}</p>
                )}
                <RemoteVideoPicker currentID={state.video?.video_id} send={send} />
              </>
            )}
            {reachable && state && mode === "chat" && <RemoteChat state={state} send={send} latestSeq={appState?.chat?.latest_seq} />}
          </>
        )}
      </div>
    </>
  );
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
