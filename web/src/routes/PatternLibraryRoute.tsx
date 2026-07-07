// Pattern Library workspace — a labeled empty state until Phase 14 delivers the
// browse/import/player/authoring/curation backend.
import { WorkspaceHead } from "../components/WorkspaceHead";

export function PatternLibraryRoute() {
  return (
    <>
      <WorkspaceHead title="Pattern library" />
      <section className="panel">
        <div className="empty-state">
          <h2>Coming in Phase 14</h2>
          <p>
            Browse, import, and enable motion patterns and programs; author new ones on a canvas; and let
            chat and Autopilot pick from the enabled set. Playback runs through the shared motion engine.
          </p>
          <p className="coming-soon">The link is here so the shell is complete; the backend lands in Phase 14.</p>
        </div>
      </section>
    </>
  );
}
