import { t } from "../i18n";
import { useCallback } from "react";
import { VideoLibrary } from "../components/VideoLibrary";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useHashRoute } from "../state/app-state";
import { videoIDFromRoute, videoRoute } from "../videos/route";

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
          canCurate={backendOnline && state?.capabilities?.control !== false}
          selectedID={videoIDFromRoute(route)}
          onSelect={select}
        />
      </div>
    </>
  );
}
