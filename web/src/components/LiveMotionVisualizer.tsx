import { useMotionState } from "../state/app-state";
import { MotionVisualizer } from "./MotionVisualizer";

// Keep the stream subscription below the conversation and its message list.
export function LiveMotionVisualizer({ mini = false }: { mini?: boolean }) {
  const motion = useMotionState();
  return <MotionVisualizer motion={motion} mini={mini} />;
}
