import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { SetupJob } from "../api/types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { SetupFailureReport } from "./SetupFailureReport";

vi.mock("../api/client", () => ({ api: { setupFailureReport: vi.fn() } }));
const job: SetupJob = { id: "failed-1", kind: "tts_update", module: "chatterbox", device: "cpu", status: "failed", message: "Dependency installation failed.", started_at: "", updated_at: "" };

describe("installation failure report", () => {
  beforeEach(() => {
    setLocaleForTest("en", english);
    vi.resetAllMocks();
    vi.stubGlobal("URL", class extends URL {
      static createObjectURL = vi.fn(() => "blob:failure-report");
      static revokeObjectURL = vi.fn();
    });
  });
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it.each(["queued", "running", "complete", "cancelled"])("does not offer a failure download for %s jobs", (status) => {
    render(<SetupFailureReport job={{ ...job, status }} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("downloads the displayed job's report in one click and releases its blob URL", async () => {
    const blob = new Blob(['{"installation":{"status":"failed"}}'], { type: "application/json" });
    let resolve!: (result: { blob: Blob; filename: string }) => void;
    vi.mocked(api.setupFailureReport).mockImplementation(() => new Promise((done) => { resolve = done; }));
    const links: Array<{ href: string; download: string }> = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      links.push({ href: this.href, download: this.download });
    });
    render(<SetupFailureReport job={job} />);
    expect(screen.getByText(/share this report with the developer/)).toBeVisible();
    expect(api.setupFailureReport).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Download failure report" }));
    expect(screen.getByRole("button", { name: "Preparing report..." })).toBeDisabled();
    await act(async () => resolve({ blob, filename: "magichandy-install-failure.json" }));
    expect(api.setupFailureReport).toHaveBeenCalledOnce();
    expect(api.setupFailureReport).toHaveBeenCalledWith(job.id);
    expect(URL.createObjectURL).toHaveBeenCalledWith(blob);
    expect(links).toEqual([{ href: "blob:failure-report", download: "magichandy-install-failure.json" }]);
    await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:failure-report"));
    expect(document.querySelector('a[download]')).toBeNull();
  });

  it("shows a recoverable download error and resets it for a replacement failure", async () => {
    vi.mocked(api.setupFailureReport).mockRejectedValue(new Error("network unavailable"));
    const view = render(<SetupFailureReport job={job} />);
    fireEvent.click(screen.getByRole("button", { name: "Download failure report" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not download the failure report. Please try again.");
    expect(screen.getByRole("button", { name: "Download failure report" })).toBeEnabled();
    view.rerender(<SetupFailureReport job={{ ...job, id: "failed-2" }} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
