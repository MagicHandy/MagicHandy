import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { NetworkConfig, NetworkStatus } from "../api/types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { NetworkSettingsPanel } from "./NetworkSettingsPanel";

vi.mock("../api/client", () => ({ api: { networkStatus: vi.fn(), validateNetwork: vi.fn(), saveNetwork: vi.fn(), networkReport: vi.fn() } }));
const config: NetworkConfig = { mode: "local", listen_address: "127.0.0.1:49717", public_url: "", tls_certificate: "", tls_private_key: "", trusted_proxies: [] };
const status: NetworkStatus = { active: config, saved: config, restart_required: false, interfaces: [], forwarded: false, authentication_required: true, secure_cookie: false };
beforeEach(() => {
  vi.resetAllMocks(); setLocaleForTest("en", english);
  vi.mocked(api.networkStatus).mockResolvedValue(status);
  vi.mocked(api.validateNetwork).mockImplementation(async (config) => ({ valid: true, config, message: "Configuration valid." }));
  vi.mocked(api.saveNetwork).mockResolvedValue({ restart_required: true });
});
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("network access settings", () => {
  it("requires host validation and password confirmation, and invalidates validation after edits", async () => {
    render(<NetworkSettingsPanel backendOnline administrator />);
    await screen.findByLabelText("Access mode");
    const save = screen.getByRole("button", { name: "Save for restart" });
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/^Current administrator password/), { target: { value: "review-only password" } });
    fireEvent.click(screen.getByRole("button", { name: "Validate configuration" }));
    await waitFor(() => expect(save).toBeEnabled());
    fireEvent.change(screen.getByLabelText("Listen address and port"), { target: { value: "127.0.0.1:49718" } });
    expect(save).toBeDisabled();
    expect(api.saveNetwork).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Validate configuration" }));
    await waitFor(() => expect(save).toBeEnabled());
    fireEvent.click(save);
    await waitFor(() => expect(api.saveNetwork).toHaveBeenCalledWith({ ...config, listen_address: "127.0.0.1:49718" }, "review-only password"));
    await waitFor(() => expect(screen.getByLabelText(/^Current administrator password/)).toHaveValue(""));
    expect(await screen.findByText(/Saved for the next app restart/)).toBeVisible();
  });

  it("does not request host settings or show editable paths to observers", () => {
    render(<NetworkSettingsPanel backendOnline administrator={false} />);
    expect(api.networkStatus).not.toHaveBeenCalled();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.getByText(/An administrator manages/)).toBeVisible();
  });

  it("downloads a support report in one click and encourages sharing it", async () => {
    vi.mocked(api.networkReport).mockResolvedValue({ report_version: 1, network_mode: "local" });
    vi.stubGlobal("URL", class extends URL { static createObjectURL = vi.fn(() => "blob:report"); static revokeObjectURL = vi.fn(); });
    const downloads: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { downloads.push(this.download); });
    render(<NetworkSettingsPanel backendOnline administrator />);
    fireEvent.click(await screen.findByRole("button", { name: "Download connection report" }));
    expect(await screen.findByText(/Connection report downloaded/)).toBeVisible();
    expect(api.networkReport).toHaveBeenCalledOnce();
    expect(downloads).toEqual(["magichandy-connection-report.json"]);
  });
});
