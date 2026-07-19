// Chat is the home workspace: assistant-driven conversation and Autopilot
// beside immediate-apply motion controls and the detailed engine visualizer.
import { AutopilotControl } from "../components/AutopilotControl";
import { ChatPanel } from "../components/ChatPanel";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { QuickSettings } from "../components/QuickSettings";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { VoiceQuickControls } from "../components/VoiceQuickControls";
import { useAppState } from "../state/app-state";

export function ChatRoute() {
  const { motion } = useAppState();
  return (
    <>
      <WorkspaceHead title="Chat" lede="Message MagicHandy — chat can start, adjust, and stop motion." wide />
      <div className="split">
        <section className="panel chat-panel-shell" aria-label="Conversation">
          <ChatPanel />
        </section>
        <aside className="panel" aria-label="Motion controls" data-requires-backend>
          <h2 className="section-title">Controls</h2>
          <AutopilotControl />
          <div className="divider" />
          <VoiceQuickControls />
          <div className="divider" />
          <h2 className="section-title">Motion behavior</h2>
          <QuickSettings section="behavior" />
          <div className="divider" />
          <h3 className="group-title">Live motion</h3>
          <MotionVisualizer motion={motion} />
        </aside>
      </div>
    </>
  );
}
