import { t } from "../i18n";
import { useCallback } from "react";
import { VideoLibrary } from "../components/VideoLibrary";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useHashRoute } from "../state/app-state";

// `#/videos/<id>` opens one video, so a reload, a bookmark or a remote command
// can address it. Going back from the player returns to `#/videos`.
export function videoRoute(id: string): string {
  return id ? `#/videos/${encodeURIComponent(id)}` : "#/videos";
}

export function videoIDFromRoute(route: string): string {
  const [base, id] = route.replace(/^#\/?/, "").split("?")[0].split("/");
  if (base !== "videos" || !id) return "";
  try {
    return decodeURIComponent(id);
  } catch {
    return "";
  }
}

export function VideoRoute() {
  const { backendOnline, readOnly, state } = useAppState();
  const route = useHashRoute();
  const select = useCallback((id: string) => {
    window.location.hash = videoRoute(id);
  }, []);

  return (
    <>
      <WorkspaceHead title={t("Videos")} wide />
      <div className="video-page" data-requires-backend>
        <VideoLibrary
          locked={!backendOnline || readOnly}
          hostAdministration={state?.capabilities?.configure_host !== false}
          stopSequence={state?.stop_sequence}
          selectedID={videoIDFromRoute(route)}
          onSelect={select}
        />
      </div>
    </>
  );
}
