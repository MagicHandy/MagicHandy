import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { ManagedSession, ManagedSessionsResponse } from "../api/types";
import { SessionSettingsPanel } from "./SessionSettingsPanel";

vi.mock("../api/client", () => ({ api: { authSessions: vi.fn(), renameSession: vi.fn(), revokeSession: vi.fn(), revokeOtherSessions: vi.fn() } }));
const list = vi.mocked(api.authSessions);
const rename = vi.mocked(api.renameSession);
const revoke = vi.mocked(api.revokeSession);
const revokeOthers = vi.mocked(api.revokeOtherSessions);
const firstID = "a".repeat(22), peerID = "b".repeat(22);
const row = (id: string, name: string, current: boolean): ManagedSession => ({ id, name, current,
  client: { browser: "chrome", platform: "windows" }, controller: false, device_gateway: false,
  created_at: "2026-09-13T12:00:00Z", last_active_at: "2026-09-13T12:05:00Z", idle_expires_at: "2026-09-13T12:35:00Z", expires_at: "2026-09-14T00:00:00Z" });
const current = row(firstID, "Current desktop", true), peer = row(peerID, "Living room tablet", false);
const response = (sessions = [current, peer]): ManagedSessionsResponse => ({ sessions, current_session_id: firstID, limit: 20 });
const onSignedOut = vi.fn();

beforeEach(() => {
  vi.clearAllMocks(); list.mockReset(); list.mockResolvedValue(response());
  rename.mockResolvedValue({ updated: true }); revoke.mockResolvedValue({ revoked: 1, current_revoked: false });
  revokeOthers.mockResolvedValue({ revoked: 1, current_revoked: false }); onSignedOut.mockResolvedValue(null);
  vi.spyOn(window, "confirm").mockReturnValue(true);
});
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

describe("own login management", () => {
  it("shows backend session facts and permits naming without controller ownership", async () => {
    render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    const peerRow = (await screen.findByText("Living room tablet")).closest("li")!;
    expect(screen.getByText("This browser")).toBeInTheDocument();
    expect(screen.getAllByText(/Idle sign-out:/)).toHaveLength(2);
    fireEvent.click(within(peerRow).getByRole("button", { name: "Name this browser" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Device name" }), { target: { value: "My tablet" } });
    list.mockResolvedValue(response([current, { ...peer, name: "My tablet" }]));
    fireEvent.click(screen.getByRole("button", { name: "Save name" }));
    expect(await screen.findByText("Session name updated.")).toBeInTheDocument();
    expect(rename).toHaveBeenCalledWith(peerID, "My tablet", expect.any(AbortSignal));
    expect(await screen.findByText("My tablet")).toBeInTheDocument();
  });

  it("asks before revoking a peer and refreshes the backend result", async () => {
    render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    const peerRow = (await screen.findByText("Living room tablet")).closest("li")!;
    vi.mocked(window.confirm).mockReturnValueOnce(false);
    fireEvent.click(within(peerRow).getByRole("button", { name: "Sign out session" }));
    expect(revoke).not.toHaveBeenCalled();
    list.mockResolvedValue(response([current]));
    fireEvent.click(within(peerRow).getByRole("button", { name: "Sign out session" }));
    await waitFor(() => expect(screen.queryByText("Living room tablet")).not.toBeInTheDocument());
    expect(revoke).toHaveBeenCalledOnce();
    expect(revoke).toHaveBeenCalledWith(peerID, expect.any(AbortSignal));
    expect(onSignedOut).not.toHaveBeenCalled();
  });

  it("reconciles a lost mutation response without replaying it", async () => {
    render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await screen.findByText("Living room tablet");
    revokeOthers.mockRejectedValueOnce(new Error("lost response"));
    list.mockResolvedValue(response([current]));
    fireEvent.click(screen.getByRole("button", { name: "Sign out other sessions" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not confirm the session change");
    await waitFor(() => expect(screen.queryByText("Living room tablet")).not.toBeInTheDocument());
    expect(revokeOthers).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Sign out other sessions" })).toBeDisabled();
  });

  it("refreshes authentication after signing out the current browser even if the reply is lost", async () => {
    render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await screen.findByText("Current desktop");
    revoke.mockRejectedValueOnce(new Error("socket closed"));
    fireEvent.click(screen.getByRole("button", { name: "Sign out this browser" }));
    await waitFor(() => expect(onSignedOut).toHaveBeenCalledOnce());
    expect(revoke).toHaveBeenCalledWith(firstID, expect.any(AbortSignal));
    expect(list).toHaveBeenCalledOnce();
  });

  it("ignores a canceled read after a completed mutation", async () => {
    let resolve!: (value: ManagedSessionsResponse) => void;
    list.mockResolvedValueOnce(response()).mockImplementationOnce(() => new Promise(done => { resolve = done; })).mockResolvedValue(response([current]));
    render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await screen.findByText("Living room tablet");
    fireEvent.click(screen.getByRole("button", { name: "Refresh sessions" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign out other sessions" }));
    await waitFor(() => expect(screen.queryByText("Living room tablet")).not.toBeInTheDocument());
    expect(list.mock.calls[1][0]?.aborted).toBe(true);
    await act(async () => resolve(response()));
    expect(screen.queryByText("Living room tablet")).not.toBeInTheDocument();
  });

  it("releases aborted action state after connectivity returns", async () => {
    revokeOthers.mockImplementationOnce(() => new Promise(() => undefined));
    const view = render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await screen.findByText("Living room tablet");
    fireEvent.click(screen.getByRole("button", { name: "Sign out other sessions" }));
    view.rerender(<SessionSettingsPanel backendOnline={false} onSignedOut={onSignedOut} />);
    expect(revokeOthers.mock.calls[0][0]?.aborted).toBe(true);
    view.rerender(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Sign out other sessions" })).toBeEnabled());
    expect(revokeOthers).toHaveBeenCalledOnce();
  });

  it("bounds a stalled session read and clears its timer on unmount", async () => {
    vi.useFakeTimers();
    list.mockImplementationOnce(signal => new Promise((_resolve, reject) => signal?.addEventListener("abort", () => reject(new Error("aborted")), { once: true })));
    const view = render(<SessionSettingsPanel backendOnline onSignedOut={onSignedOut} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(10_001); });
    expect(screen.getByRole("alert")).toHaveTextContent("Session request timed out");
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
