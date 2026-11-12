import { isHandy, playbackDevices } from "../api/devices";
import type { PublicSettings } from "../api/types";
import { useAppState, useMotionState } from "../state/app-state";
import { LiveVibrationReadout, showsVibration } from "./VibrationReadout";
import { MotionVisualizer } from "./MotionVisualizer";
import { StartupRecoveryNotice } from "./StartupRecoveryNotice";
import type { VisualizerDevice } from "./visualizer/device";

/** The dispatch owner and saved Handy model, which pick the visualizer drawing.
 * Playback with an Autoblow device and no Handy draws a generic track. */
export function visualizerDeviceFor(settings: PublicSettings | undefined): VisualizerDevice {
  const playing = playbackDevices(settings?.device);
  const generic = !playing.some(isHandy) && playing.some((id) => id === "autoblow" || id === "vacuglide");
  return { owner: settings?.device?.hsp_dispatch_owner, model: settings?.motion?.handy_model, generic };
}

export function useVisualizerDevice(): VisualizerDevice {
  const { state } = useAppState();
  return visualizerDeviceFor(state?.settings);
}

// Keep the stream subscription below the conversation and its message list.
export function LiveMotionVisualizer({ mini = false }: { mini?: boolean }) {
  const motion = useMotionState();
  const device = useVisualizerDevice();
  const { state } = useAppState();
  if (showsVibration(state?.settings, motion)) return <LiveVibrationReadout mini={mini} />;
  return <><MotionVisualizer motion={motion} mini={mini} device={device} />{!mini && <StartupRecoveryNotice announce />}</>;
}
