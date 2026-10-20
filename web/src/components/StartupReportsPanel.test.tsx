import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { StartupReport } from "../api/startup-types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { StartupReportsPanel } from "./StartupReportsPanel";

vi.mock("../api/client", () => ({ api: { startupReports: vi.fn() } }));
const report: StartupReport = { schema_version: 1, app_version: "test", commit: "test", platform: "windows/amd64", started_at: "2026-10-20T12:00:00Z", finished_at: "2026-10-20T12:00:01Z", stage: "ready", outcome: "recovered", attempts: [{ listener: "app", port: 49717, error_code: "permission_denied", native_code: 10013 }, { listener: "app", port: 48800 }] };
beforeEach(() => { vi.resetAllMocks(); setLocaleForTest("en", english); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it("shows recovery and retained failures, and downloads the backend report", async () => {
  const reports = { current: report, last_failure: { ...report, outcome: "failed" }, last_recovery: report };
  vi.mocked(api.startupReports).mockResolvedValue(reports as never);
  const create = vi.fn(() => "blob:startup-report");
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: vi.fn() });
  let downloaded = "";
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function(this: HTMLAnchorElement) { downloaded = this.download; });
  render(<StartupReportsPanel />);
  expect(await screen.findByText("MagicHandy selected an available local port automatically.")).toBeVisible();
  expect(screen.getByText(/A previous startup failure was retained/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Download startup report" }));
  expect(create).toHaveBeenCalledWith(expect.any(Blob));
  expect(click).toHaveBeenCalledOnce();
  expect(downloaded).toBe("magichandy-startup-report.json");
  await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:startup-report"));
});

it("explains how to find reports when the API cannot return them", async () => {
  vi.mocked(api.startupReports).mockRejectedValue(new Error("offline"));
  render(<StartupReportsPanel />);
  expect(await screen.findByRole("status")).toHaveTextContent("Check the startup folder in the app data directory.");
  expect(screen.queryByRole("button", { name: "Download startup report" })).toBeNull();
});
