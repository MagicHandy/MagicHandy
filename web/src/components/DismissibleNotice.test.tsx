import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { setLocaleForTest } from "../i18n";
import type { NoticePreferences } from "../notice-catalog";
import { NoticePreferencesProvider } from "../state/notice-preferences";
import { DismissibleNotice } from "./DismissibleNotice";
import { NoticePreferencesPanel } from "./NoticePreferencesPanel";

vi.mock("../api/client", () => ({ api: { noticePreferences: vi.fn(), saveNoticePreference: vi.fn(), resetNoticePreferences: vi.fn() } }));
const read = vi.mocked(api.noticePreferences), save = vi.mocked(api.saveNoticePreference), reset = vi.mocked(api.resetNoticePreferences);
const snapshot = (hidden: string[] = [], scope: "account" | "browser" = "browser"): NoticePreferences => ({ scope, hidden });
function Fixture({ audience = "browser", account = false, settings = false }: { audience?: string; account?: boolean; settings?: boolean }) {
  return <NoticePreferencesProvider key={audience} enabled scope={account ? "account" : "browser"}>
    <DismissibleNotice id="sign-in-safety"><strong>Emergency Stop remains available.</strong><span>Informational explanation</span></DismissibleNotice>
    {settings && <NoticePreferencesPanel />}
  </NoticePreferencesProvider>;
}
const open = () => fireEvent.click(screen.getByRole("button", { name: "Dismiss Emergency Stop remains available." }));
beforeEach(() => { vi.resetAllMocks(); setLocaleForTest("en"); read.mockResolvedValue(snapshot()); save.mockResolvedValue(snapshot(["sign-in-safety"])); reset.mockResolvedValue(snapshot()); });
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("shared informational notices", () => {
  it("asks for the dismissal lifetime and restores keyboard focus when canceled", async () => {
    render(<Fixture />);
    expect(await screen.findByRole("note")).toBeInTheDocument();
    open();
    expect(screen.getByText("Don't show again will be saved for this browser.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Just this time" })).toHaveFocus();
    const emergencyKey = vi.fn();
    window.addEventListener("keydown", emergencyKey);
    fireEvent.keyDown(screen.getByRole("group"), { key: "Escape" });
    window.removeEventListener("keydown", emergencyKey);
    expect(emergencyKey).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Dismiss Emergency Stop remains available." })).toHaveFocus();
    expect(save).not.toHaveBeenCalled();
  });

  it("dismisses for this visit without creating a persistent record", async () => {
    const view = render(<Fixture />);
    await screen.findByRole("note"); open();
    fireEvent.click(screen.getByRole("button", { name: "Just this time" }));
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
    expect(view.container).toBeEmptyDOMElement();
    view.rerender(<Fixture settings />);
    expect(screen.getByText("Just this time")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show again" }));
    expect(screen.getByRole("note")).toHaveTextContent("Informational explanation");
    expect(save).not.toHaveBeenCalled();
    open(); fireEvent.click(screen.getByRole("button", { name: "Just this time" }));
    view.unmount(); render(<Fixture />);
    expect(await screen.findByRole("note")).toHaveTextContent("Informational explanation");
  });

  it("leaves no notice marker after durable confirmation and restores only in settings", async () => {
    let complete!: (value: NoticePreferences) => void;
    save.mockImplementationOnce(() => new Promise(resolve => { complete = resolve; }));
    const view = render(<Fixture />);
    await screen.findByRole("note"); open();
    fireEvent.click(screen.getByRole("button", { name: "Don't show again" }));
    expect(screen.getByRole("note")).toBeInTheDocument();
    expect(save).toHaveBeenCalledWith("sign-in-safety", true, "browser", expect.any(AbortSignal));
    await act(async () => complete(snapshot(["sign-in-safety"])));
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
    expect(screen.queryByText("Emergency Stop remains available.")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    view.rerender(<Fixture settings />);
    save.mockResolvedValueOnce(snapshot());
    fireEvent.click(screen.getByRole("button", { name: "Show again" }));
    expect(await screen.findByRole("note")).toHaveTextContent("Informational explanation");
    expect(screen.queryByRole("group", { name: "Hide this notice?" })).not.toBeInTheDocument();
    expect(save).toHaveBeenLastCalledWith("sign-in-safety", false, "browser", expect.any(AbortSignal));
  });

  it("keeps a notice visible when saving fails", async () => {
    save.mockRejectedValueOnce(new Error("storage unavailable"));
    render(<Fixture />); await screen.findByRole("note"); open();
    fireEvent.click(screen.getByRole("button", { name: "Don't show again" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("storage unavailable");
    expect(screen.getByRole("note")).toBeInTheDocument();
  });

  it("bounds a stalled save and ignores success arriving after the deadline", async () => {
    vi.useFakeTimers();
    let complete!: (value: NoticePreferences) => void;
    save.mockImplementationOnce(() => new Promise(resolve => { complete = resolve; }));
    render(<Fixture />); await act(async () => undefined); open();
    fireEvent.click(screen.getByRole("button", { name: "Don't show again" }));
    await act(async () => { await vi.advanceTimersByTimeAsync(5001); });
    expect(screen.getByRole("alert")).toHaveTextContent("Refresh to check the saved state");
    expect(save.mock.calls[0][3]?.aborted).toBe(true);
    await act(async () => complete(snapshot(["sign-in-safety"])));
    expect(screen.getByRole("note")).toBeInTheDocument();
    expect(save).toHaveBeenCalledOnce();
  });

  it("retires pending observations when the account changes", async () => {
    let completeOld!: (value: NoticePreferences) => void;
    read.mockImplementationOnce(() => new Promise(resolve => { completeOld = resolve; })).mockResolvedValueOnce(snapshot([], "account"));
    const view = render(<Fixture audience="alice" account />);
    const oldSignal = read.mock.calls[0][1];
    view.rerender(<Fixture audience="bob" account />);
    expect(await screen.findByRole("note")).toBeInTheDocument();
    expect(oldSignal?.aborted).toBe(true);
    await act(async () => completeOld(snapshot(["sign-in-safety"], "account")));
    expect(screen.getByRole("note")).toBeInTheDocument();
  });

  it("restores account notices and the browser sign-in notice through one manager", async () => {
    read.mockResolvedValueOnce(snapshot(["model-generation", "sign-in-safety"], "account"));
    reset.mockResolvedValueOnce(snapshot([], "account"));
    render(<NoticePreferencesProvider enabled scope="account"><NoticePreferencesPanel /></NoticePreferencesProvider>);
    expect(await screen.findByText("Generation guidance")).toBeInTheDocument();
    expect(screen.getByText("This browser")).toBeInTheDocument();
    expect(screen.getByText("Your account")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show all notices again" }));
    await waitFor(() => expect(screen.getByText("All informational notices are shown.")).toBeInTheDocument());
    expect(reset).toHaveBeenCalledWith("account", expect.any(AbortSignal));
  });
});
