import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { IssuedRecoveryCodes, RecoveryCodeStatus } from "../api/access-types";
import { RecoveryCodesPanel } from "./RecoveryCodesPanel";

vi.mock("../api/client", () => ({ api: { recoveryCodeStatus: vi.fn(), replaceRecoveryCodes: vi.fn(), removeRecoveryCodes: vi.fn() } }));
const read = vi.mocked(api.recoveryCodeStatus), replace = vi.mocked(api.replaceRecoveryCodes), remove = vi.mocked(api.removeRecoveryCodes);
const empty: RecoveryCodeStatus = { remaining: 0, limit: 8 };
const issued: IssuedRecoveryCodes = { remaining: 8, limit: 8, created_at: "2026-09-13T12:00:00Z",
  codes: Array.from({ length: 8 }, (_, i) => `000${i}-ABCD-ABCD-ABCD-ABCD-ABCD-ABCD-ABCD`) };
const expand = () => fireEvent.click(screen.getByRole("button", { name: "Recovery codes" }));
const generate = () => {
  fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "synthetic password" } });
  fireEvent.click(screen.getByRole("button", { name: "Generate new recovery codes" }));
};
beforeEach(() => { vi.resetAllMocks(); read.mockResolvedValue(empty); replace.mockResolvedValue(issued); remove.mockResolvedValue(empty); });
afterEach(() => vi.useRealTimers());

describe("saved account recovery codes", () => {
  it("loads only when opened and hides saved secrets without retrieving them again", async () => {
    render(<RecoveryCodesPanel backendOnline />);
    expect(read).not.toHaveBeenCalled();
    expand();
    await screen.findByText("0 of 8 recovery codes available.");
    generate();
    await screen.findByText(issued.codes[0]);
    expect(replace).toHaveBeenCalledWith("synthetic password", expect.any(AbortSignal));
    expect(screen.getByLabelText("Current password")).toHaveValue("");
    fireEvent.click(screen.getByRole("button", { name: "I saved the codes" }));
    expect(screen.queryByText(issued.codes[0])).not.toBeInTheDocument();
    read.mockResolvedValue({ remaining: 8, limit: 8, created_at: issued.created_at });
    fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
    expect(screen.queryByText(issued.codes[0])).not.toBeInTheDocument();
    expect(replace).toHaveBeenCalledOnce();
  });

  it("discards displayed codes when a refreshed set has changed on another browser", async () => {
    render(<RecoveryCodesPanel backendOnline />); expand();
    await screen.findByText("0 of 8 recovery codes available."); generate();
    await screen.findByText(issued.codes[0]);
    read.mockResolvedValue({ remaining: 8, limit: 8, created_at: "2026-09-13T12:01:00Z" });
    fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
    await waitFor(() => expect(screen.queryByText(issued.codes[0])).not.toBeInTheDocument());
  });

  it("cancels offline work and ignores a late issuance after reconnect", async () => {
    let resolve!: (result: IssuedRecoveryCodes) => void;
    replace.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    const view = render(<RecoveryCodesPanel backendOnline />); expand();
    await screen.findByText("0 of 8 recovery codes available."); generate();
    view.rerender(<RecoveryCodesPanel backendOnline={false} />);
    expect(replace.mock.calls[0][1]?.aborted).toBe(true);
    expect(screen.getByLabelText("Current password")).toHaveValue("");
    view.rerender(<RecoveryCodesPanel backendOnline />);
    await screen.findByText("0 of 8 recovery codes available.");
    await act(async () => resolve(issued));
    expect(screen.queryByText(issued.codes[0])).not.toBeInTheDocument();
    expect(screen.getByText("0 of 8 recovery codes available.")).toBeInTheDocument();
  });

  it("bounds stalled work even if cancellation is ignored and clears timers when closed", async () => {
    vi.useFakeTimers();
    let resolve!: (result: RecoveryCodeStatus) => void;
    read.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    const view = render(<RecoveryCodesPanel backendOnline />); expand();
    await act(async () => { await vi.advanceTimersByTimeAsync(10_001); });
    expect(read.mock.calls[0][0]?.aborted).toBe(true);
    expect(screen.getByRole("alert")).toHaveTextContent("Refresh its status before trying again");
    expect(screen.getByRole("button", { name: "Refresh status" })).toBeEnabled();
    await act(async () => resolve(empty));
    expect(screen.queryByText("0 of 8 recovery codes available.")).not.toBeInTheDocument();
    expand(); view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("rejects malformed issuance and removes the saved set with password confirmation", async () => {
    replace.mockResolvedValueOnce({ ...issued, codes: ["unexpected server content"] });
    render(<RecoveryCodesPanel backendOnline />); expand();
    await screen.findByText("0 of 8 recovery codes available."); generate();
    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid recovery response");
    expect(screen.queryByText("unexpected server content")).not.toBeInTheDocument();
    generate(); await screen.findByText(issued.codes[0]);
    fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "synthetic password" } });
    fireEvent.click(screen.getByRole("button", { name: "Remove recovery codes" }));
    await screen.findByText("0 of 8 recovery codes available.");
    expect(remove).toHaveBeenCalledWith("synthetic password", expect.any(AbortSignal));
    expect(screen.queryByText(issued.codes[0])).not.toBeInTheDocument();
  });
});
