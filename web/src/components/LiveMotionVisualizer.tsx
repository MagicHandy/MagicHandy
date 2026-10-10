import { useAppState, useMotionState } from "../state/app-state";
import { MotionVisualizer } from "./MotionVisualizer";
import type { VisualizerDevice } from "./visualizer/device";

/** The dispatch owner and saved Handy model, which pick the visualizer drawing. */
export function useVisualizerDevice(): VisualizerDevice {
  const { state } = useAppState();
  return { owner: state?.settings?.device?.hsp_dispatch_owner, model: state?.settings?.motion?.handy_model };
}

// Keep the stream subscription below the conversation and its message list.
export function LiveMotionVisualizer({ mini = false }: { mini?: boolean }) {
  const motion = useMotionState();
  const device = useVisualizerDevice();
  return <MotionVisualizer motion={motion} mini={mini} device={device} />;
}
