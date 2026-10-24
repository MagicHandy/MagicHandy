import { api } from "../api/client";
import { EdgeControl } from "../shell/EdgeButton";
import type { RemoteView } from "./useRemoteState";

// The phone's Edge acts on the desktop's session directly (ADR 0055), so it
// works while the desktop tab is hidden. It needs a live session of this
// account and the current Emergency Stop sequence; Stop stays its sibling.
export function RemoteEdgeButton({ remote }: { remote: RemoteView }) {
  const state = remote.state;
  const session = state?.session;
  const sequence = state?.stop_sequence;
  const unavailable = remote.stale || !session?.alive || sequence === undefined || Boolean(state?.other_account);
  return <EdgeControl className="remote-edge" hold={session?.edge} allowed={Boolean(session?.edge_allowed)} unavailable={unavailable}
    activate={() => api.remoteEdge(sequence ?? 0)} resume={() => api.remoteEdgeResume(sequence ?? 0)} onSettled={remote.refresh} />;
}
