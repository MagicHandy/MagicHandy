// Chat is the home workspace: the conversation beside a control column with the
// immediate-apply quick settings, the testing-badged manual motion group, and
// the detailed engine visualizer.
import { ChatPanel } from "../components/ChatPanel";
import { ManualMotionTest } from "../components/ManualMotionTest";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { QuickSettings } from "../components/QuickSettings";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { VoiceQuickControls } from "../components/VoiceQuickControls";
import { useAppState } from "../state/app-state";

export function ChatRoute() {
  const { motion } = useAppState();
  return (
    <>
      <WorkspaceHead title="Chat" lede="Message MagicHandy — chat can start, adjust, and stop motion." />
      <div className="split">
        <section className="panel chat-panel-shell" aria-label="Conversation">
          <ChatPanel />
        </section>
        <aside className="panel" aria-label="Motion controls" data-requires-backend>
          <h2 className="section-title">Controls</h2>
          <VoiceQuickControls />
          <div className="divider" />
          <h2 className="section-title">
            Quick settings <span className="hint-inline">applies immediately</span>
          </h2>
          <QuickSettings />
          <div className="divider" />
          <ManualMotionTest />
          <div className="divider" />
          <h3 className="group-title">Live motion</h3>
          <MotionVisualizer motion={motion} />
        </aside>
      </div>
    </>
  );
}
