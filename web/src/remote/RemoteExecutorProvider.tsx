import { createContext, useContext, useEffect, useLayoutEffect, useMemo, useState, type ReactNode } from "react";
import { api, REMOTE_COMMAND_EVENT } from "../api/client";
import type { RemoteCommand } from "../api/remote-types";
import { useAppState } from "../state/app-state";
import { RemoteExecutor, type RemoteChatSurface, type RemoteVideoSurface } from "./executor";

interface RemoteSurfaces {
  setVideo: (surface: RemoteVideoSurface | null) => void;
  setChat: (surface: RemoteChatSurface | null) => void;
}

// Outside the provider (isolated component tests) registration does nothing.
const RemoteSurfaceContext = createContext<RemoteSurfaces>({ setVideo: () => undefined, setChat: () => undefined });

// Makes this tab the phone remote's desktop while it holds control and is in
// front. Pages register what they show; commands arrive on the motion stream.
export function RemoteExecutorProvider({ route, children }: { route: string; children: ReactNode }) {
  const { backendOnline, readOnly, state } = useAppState();
  const visible = useDocumentVisible();
  const executor = useMemo(() => new RemoteExecutor({
    claim: (id, stopSequence) => api.claimRemoteCommand(id, stopSequence),
    report: (presence) => api.reportRemotePresence(presence),
    withdraw: () => void api.withdrawRemotePresence().catch(() => undefined),
    navigate: (hash) => {
      window.location.hash = hash;
    },
  }), []);
  const eligible = backendOnline && !readOnly && state?.capabilities?.control !== false && visible;

  // Unmounting only steps back, so StrictMode's remount resumes the same executor.
  useLayoutEffect(() => {
    executor.setAdmission(state?.stop_sequence, `${state?.controller?.epoch ?? ""}:${state?.controller?.generation ?? ""}`);
  }, [executor, state?.stop_sequence, state?.controller?.epoch, state?.controller?.generation]);
  useLayoutEffect(() => {
    executor.setEligible(eligible);
    return () => executor.setEligible(false);
  }, [executor, eligible]);
  useEffect(() => executor.setRoute(route), [executor, route]);
  useEffect(() => {
    const receive = (event: Event) => void executor.execute((event as CustomEvent<RemoteCommand>).detail);
    const stop = () => executor.cancelPending();
    window.addEventListener(REMOTE_COMMAND_EVENT, receive);
    window.addEventListener("magichandy:emergency-stop", stop);
    return () => {
      window.removeEventListener(REMOTE_COMMAND_EVENT, receive);
      window.removeEventListener("magichandy:emergency-stop", stop);
    };
  }, [executor]);

  const surfaces = useMemo<RemoteSurfaces>(() => ({
    setVideo: (surface) => executor.setVideo(surface),
    setChat: (surface) => executor.setChat(surface),
  }), [executor]);
  return <RemoteSurfaceContext.Provider value={surfaces}>{children}</RemoteSurfaceContext.Provider>;
}

/** Offers the open player to the remote. Pass a memoized surface. */
export function useRemoteVideoSurface(surface: RemoteVideoSurface | null): void {
  const { setVideo } = useContext(RemoteSurfaceContext);
  useEffect(() => {
    setVideo(surface);
    return () => setVideo(null);
  }, [setVideo, surface]);
}

/** Offers the open conversation to the remote. Pass a memoized surface. */
export function useRemoteChatSurface(surface: RemoteChatSurface | null): void {
  const { setChat } = useContext(RemoteSurfaceContext);
  useEffect(() => {
    setChat(surface);
    return () => setChat(null);
  }, [setChat, surface]);
}

function useDocumentVisible(): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState !== "hidden");
  useEffect(() => {
    const update = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", update);
    window.addEventListener("pageshow", update);
    return () => {
      document.removeEventListener("visibilitychange", update);
      window.removeEventListener("pageshow", update);
    };
  }, []);
  return visible;
}
