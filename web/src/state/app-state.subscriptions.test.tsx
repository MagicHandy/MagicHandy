import { Profiler, type ProfilerOnRenderCallback } from "react";
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AppState } from "../api/types";
import { AppStateProvider, useAppState, useMotionState } from "./app-state";

vi.mock("../api/client", () => ({
  clientId: "subscription-profile",
  api: { getState: vi.fn(async () => ({ controller: { read_only: false }, motion: { available: true } }) as AppState) },
}));

class EventSourceFixture {
  static source: EventSourceFixture;
  listener?: EventListener;
  close = vi.fn();
  constructor() { EventSourceFixture.source = this; }
  addEventListener(_event: string, listener: EventListener) { this.listener = listener; }
  emit(position: number) {
    this.listener?.(new MessageEvent("motion", { data: JSON.stringify({ available: true, engine: { running: true, position_percent: position } }) }));
  }
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("does not rerender a large conversation or library on motion-only events", async () => {
  vi.stubGlobal("EventSource", EventSourceFixture);
  const renders = { conversation: 0, library: 0 };
  let liveRenders = 0;
  let commits = 0;
  let duration = 0;
  const onRender: ProfilerOnRenderCallback = (_id, _phase, actualDuration) => { commits++; duration += actualDuration; };
  function LiveIndicator() {
    const motion = useMotionState();
    liveRenders++;
    return <output>{String(motion?.engine?.running)}</output>;
  }
  function SlowView({ kind }: { kind: keyof typeof renders }) {
    const { readOnly } = useAppState();
    renders[kind]++;
    return <div aria-readonly={readOnly}>{Array.from({ length: 500 }, (_, index) => <p key={index}>{kind} row {index}</p>)}</div>;
  }
  render(<AppStateProvider><LiveIndicator /><Profiler id="large-views" onRender={onRender}><SlowView kind="conversation" /><SlowView kind="library" /></Profiler></AppStateProvider>);
  await act(async () => {});
  const baseline = { ...renders };
  const baselineLive = liveRenders;
  commits = 0;
  duration = 0;
  for (let i = 0; i < 80; i++) act(() => EventSourceFixture.source.emit(i));
  if (import.meta.env.VITE_PROFILE_SUBSCRIPTIONS) console.info(JSON.stringify({ motionEvents: 80, rowsPerView: 500, extraRenders: { conversation: renders.conversation - baseline.conversation, library: renders.library - baseline.library }, commits, duration }));
  expect(renders).toEqual(baseline);
  expect(liveRenders - baselineLive).toBe(80);
});
