import type { ComponentProps } from "react";
import { useMotionState } from "../state/app-state";
import { ProgramLibrary } from "./ProgramLibrary";

// Playback progress does not invalidate the sibling pattern browser/editor.
export function LiveProgramLibrary(props: Omit<ComponentProps<typeof ProgramLibrary>, "engine">) {
  const motion = useMotionState();
  return <ProgramLibrary {...props} engine={motion?.engine} />;
}
