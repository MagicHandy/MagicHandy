import { act, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { RemoteState } from "../api/remote-types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { RemoteEdgeButton } from "./RemoteEdgeButton";
import { useRemoteKeepalive } from "./useRemoteKeepalive";
import type { RemoteView } from "./useRemoteState";

const toast = vi.hoisted(() => ({ show: vi.fn() }));
vi.mock("../state/app-state", () => ({ useToast: () => toast }));
vi.mock("../api/client", async (original) => ({ ...await original<typeof import("../api/client")>(), api: {
  remoteEdge: vi.fn(async () => ({ session: { alive: true } })),
  remoteEdgeResume: vi.fn(async () => ({ session: { alive: true } })),
  remoteKeepalive: vi.fn(async () => ({ keepalive: { renewed: true } })),
} }));

function view(state: Partial<RemoteState> | null, patch: Partial<RemoteView> = {}): RemoteView {
  return {
    state: state ? { revision: 1, connected: false, pending: 0, recent: [], stop_sequence: 7, ...state } : null,
    receivedAt: 0, stale: false, error: "", refresh: vi.fn(), ...patch,
  };
}

describe("phone Edge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setLocaleForTest("en", english);
  });

  it("holds and returns a hidden desktop's session with the current Stop sequence", async () => {
    const refresh = vi.fn();
    const live = { alive: true, running: true, edge_allowed: true };
    const rendered = render(<RemoteEdgeButton remote={view({ session: live }, { refresh })} />);
    fireEvent.click(screen.getByRole("button", { name: "Edge — gentle hold" }));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(api.remoteEdge).toHaveBeenCalledWith(7);
    expect(screen.queryByRole("button", { name: "Resume from gentle hold" })).toBeNull();

    rendered.rerender(<RemoteEdgeButton remote={view({ session: { ...live, edge: { active: true, phase: "holding", automatic: false } } }, { refresh })} />);
    expect(screen.getByRole("status")).toHaveTextContent("Holding until you resume");
    fireEvent.click(screen.getByRole("button", { name: "Resume from gentle hold" }));
    await waitFor(() => expect(api.remoteEdgeResume).toHaveBeenCalledWith(7));
  });

  it.each(["no session", "stale", "other account", "no live motion"])("disables Edge with %s", (kind) => {
    const state: Partial<RemoteState> = { session: { alive: kind !== "no session", running: true, edge_allowed: kind !== "no live motion" } };
    if (kind === "other account") state.other_account = true;
    render(<RemoteEdgeButton remote={view(state, { stale: kind === "stale" })} />);
    expect(screen.getByRole("button", { name: "Edge — gentle hold" })).toBeDisabled();
  });

  it("reports a refused press without changing the view", async () => {
    vi.mocked(api.remoteEdge).mockRejectedValueOnce(new Error("no desktop session of this account holds control"));
    render(<RemoteEdgeButton remote={view({ session: { alive: true, running: true, edge_allowed: true } })} />);
    fireEvent.click(screen.getByRole("button", { name: "Edge — gentle hold" }));
    await waitFor(() => expect(toast.show).toHaveBeenCalledWith(expect.any(String), "error"));
  });
});

describe("phone keepalive", () => {
  let visibility: DocumentVisibilityState = "visible";

  beforeEach(() => {
    vi.clearAllMocks();
    visibility = "visible";
    vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("renews while the page is in front and goes quiet while it is hidden", async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useRemoteKeepalive(true));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(api.remoteKeepalive).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(api.remoteKeepalive).toHaveBeenCalledTimes(2);

    visibility = "hidden";
    act(() => { document.dispatchEvent(new Event("visibilitychange")); });
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(api.remoteKeepalive).toHaveBeenCalledTimes(2);

    visibility = "visible";
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(api.remoteKeepalive).toHaveBeenCalledTimes(3);
  });

  it("reports a session that could not be renewed", async () => {
    vi.mocked(api.remoteKeepalive).mockResolvedValueOnce({ keepalive: { renewed: false } });
    const { result } = renderHook(() => useRemoteKeepalive(true));
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("sends nothing without control permission", () => {
    renderHook(() => useRemoteKeepalive(false));
    expect(api.remoteKeepalive).not.toHaveBeenCalled();
  });
});
