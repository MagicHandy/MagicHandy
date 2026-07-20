import { VideoLibrary } from "../components/VideoLibrary";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState } from "../state/app-state";

export function VideoRoute() {
  const { backendOnline, readOnly } = useAppState();

  return (
    <>
      <WorkspaceHead title="Videos" wide />
      <div className="video-page" data-requires-backend>
        <VideoLibrary locked={!backendOnline || readOnly} />
      </div>
    </>
  );
}
