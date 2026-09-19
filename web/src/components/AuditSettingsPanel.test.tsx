import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AccessAuditPage } from "../api/audit-types";
import { AuditSettingsPanel } from "./AuditSettingsPanel";

vi.mock("../api/client", () => ({ api: { accessAudit: vi.fn(), exportAccessAudit: vi.fn() } }));
const list = vi.mocked(api.accessAudit), download = vi.mocked(api.exportAccessAudit);
const downloadedNames: string[] = [];
function page(sequence = 120, more = true): AccessAuditPage {
  return { events: [{ id: `event-${sequence}`, sequence, occurred_at_ms: 1789300000000, kind: "control_claimed", outcome: "success", actor: { type: "account", account_id: "a".repeat(32) }, generation: 3 }],
    newest_sequence: 120, oldest_sequence: 1, next_before: sequence, has_more: more, retention_days: 30, limit: 100, row_limit: 10000,
    writer: { queue_depth: 0, queue_limit: 320, dropped_since_startup: 0, write_failures_since_startup: 0, storage_available: true, last_write_ms: 1789300000000 } };
}
function toggle(open: boolean) {
  const details = screen.getByText("Access and control history").closest("details")!;
  details.open = open;
  fireEvent(details, new Event("toggle"));
}
beforeEach(() => {
  vi.clearAllMocks(); list.mockReset(); download.mockReset();
  list.mockResolvedValue(page()); download.mockResolvedValue(page());
  vi.stubGlobal("URL", class extends URL { static createObjectURL = vi.fn(() => "blob:audit-report"); static revokeObjectURL = vi.fn(); });
  downloadedNames.length = 0;
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function(this: HTMLAnchorElement) { downloadedNames.push(this.download); });
});
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("administrator access history", () => {
  it("opens a dedicated history page immediately and labels recovery code counts accurately", async () => {
    const result = page();
    result.events[0] = { ...result.events[0], kind: "recovery_codes_replaced", count: 8 };
    list.mockResolvedValueOnce(result);
    render(<AuditSettingsPanel backendOnline accounts={[]} initiallyExpanded />);
    expect(await screen.findByText("Recovery codes replaced")).toBeInTheDocument();
    expect(screen.getByText(/8 recovery codes/)).toBeInTheDocument();
    expect(screen.queryByText(/8 sessions/)).not.toBeInTheDocument();
  });

  it("loads only while expanded and replaces pages instead of retaining an unbounded feed", async () => {
    render(<AuditSettingsPanel backendOnline accounts={[]} />);
    expect(list).not.toHaveBeenCalled();
    toggle(true);
    expect(await screen.findByText("Control claimed")).toBeInTheDocument();
    expect(screen.getByText("Account aaaaaaaa")).toBeInTheDocument();
    expect(list).toHaveBeenCalledWith(0, expect.any(AbortSignal));
    list.mockResolvedValueOnce(page(20, false));
    fireEvent.click(screen.getByRole("button", { name: "Older events" }));
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(120, expect.any(AbortSignal)));
    expect(within(screen.getByRole("list", { name: "Access events" })).getAllByRole("listitem")).toHaveLength(1);
    expect(screen.getByText("20 · event-20")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Older events" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Newest events" }));
    await screen.findByText("120 · event-120");
  });

  it("downloads the viewed page with a new authorized request and releases the blob URL", async () => {
    render(<AuditSettingsPanel backendOnline accounts={[]} />); toggle(true);
    await screen.findByText("Control claimed");
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole("button", { name: "Download this page" }));
    await act(async () => undefined);
    expect(download).toHaveBeenCalledWith(121, expect.any(AbortSignal));
    expect(URL.createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledOnce();
    expect(downloadedNames).toEqual(["magichandy-access-history.json"]);
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:audit-report");
  });

  it("ignores stale pages after disconnect and cancels an export when the panel closes", async () => {
    let finishRead!: (value: AccessAuditPage) => void, finishDownload!: (value: AccessAuditPage) => void;
    list.mockImplementationOnce(() => new Promise(resolve => { finishRead = resolve; }));
    const view = render(<AuditSettingsPanel backendOnline accounts={[]} />); toggle(true);
    const readSignal = list.mock.calls[0][1]!;
    view.rerender(<AuditSettingsPanel backendOnline={false} accounts={[]} />);
    expect(readSignal.aborted).toBe(true);
    await act(async () => finishRead(page(2)));
    expect(screen.queryByText("Control claimed")).not.toBeInTheDocument();
    view.rerender(<AuditSettingsPanel backendOnline accounts={[]} />);
    await screen.findByText("Control claimed");
    download.mockImplementationOnce(() => new Promise(resolve => { finishDownload = resolve; }));
    fireEvent.click(screen.getByRole("button", { name: "Download this page" }));
    const exportSignal = download.mock.calls[0][1]!;
    toggle(false);
    expect(exportSignal.aborted).toBe(true);
    await act(async () => finishDownload(page()));
    expect(HTMLAnchorElement.prototype.click).not.toHaveBeenCalled();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });

  it("bounds a stalled read and supports retry without accepting its late result", async () => {
    vi.useFakeTimers();
    let finish!: (value: AccessAuditPage) => void;
    list.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const view = render(<AuditSettingsPanel backendOnline accounts={[]} />); toggle(true);
    await act(async () => vi.advanceTimersByTimeAsync(10001));
    expect(screen.getByRole("alert")).toHaveTextContent("Access history is unavailable.");
    expect(list.mock.calls[0][1]?.aborted).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Refresh history" }));
    await act(async () => undefined);
    await act(async () => finish(page(2)));
    expect(screen.getByText("120 · event-120")).toBeInTheDocument();
    expect(screen.queryByText("2 · event-2")).not.toBeInTheDocument();
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("shows explicit loss and unconfirmed Stop outcomes", async () => {
    const result = page(); result.writer.dropped_since_startup = 7;
    result.events[0] = { ...result.events[0], kind: "stop_finished", outcome: "unconfirmed", actor: { type: "public" } };
    list.mockResolvedValue(result);
    render(<AuditSettingsPanel backendOnline accounts={[]} />); toggle(true);
    expect(await screen.findByRole("alert")).toHaveTextContent("7 events were dropped");
    expect(screen.getByText("Stop result")).toBeInTheDocument();
    expect(screen.getByText("Unconfirmed")).toBeInTheDocument();
    expect(screen.getByText("Unauthenticated caller")).toBeInTheDocument();
  });
});
