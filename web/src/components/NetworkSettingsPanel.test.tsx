import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { NetworkConfig, NetworkStatus } from "../api/types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { NetworkSettingsPanel } from "./NetworkSettingsPanel";

vi.mock("../api/client", () => ({ api: { networkStatus: vi.fn(), validateNetwork: vi.fn(), saveNetwork: vi.fn(), networkReport: vi.fn(), discoverInternet: vi.fn(), prepareNetworkCertificate: vi.fn(), certificatePreparation: vi.fn() } }));
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
    await screen.findByRole("radio", { name: /Local only/ });
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

  it("detects the public address, shows exact forwarding ports, and never saves failed issuance", async () => {
    vi.mocked(api.networkStatus).mockResolvedValue({ ...status, interfaces: [{ name: "Ethernet", address: "192.168.1.10", loopback: false, private: true }] });
    vi.mocked(api.discoverInternet).mockResolvedValue({ public_ip: "8.8.8.8", terms_url: "https://letsencrypt.org/documents/test-terms.pdf", ip_error: false, ca_error: false, external_port: 443 });
    vi.mocked(api.prepareNetworkCertificate).mockResolvedValue({ state: "failed", message: "Public TCP 443 did not reach this computer." });
    render(<NetworkSettingsPanel backendOnline administrator />);
    fireEvent.click(await screen.findByRole("radio", { name: /^Public/ }));
    await waitFor(() => expect(screen.getByLabelText(/^HTTPS address/)).toHaveValue("https://8.8.8.8"));
    expect(screen.getByText(/Open incoming TCP 443.*192\.168\.1\.10:49717.*TCP 49717/)).toBeVisible();
    expect(screen.getByText(/Port 80 is not needed/)).toBeVisible();
    expect(screen.getByText(/does not prove that incoming connections/)).toBeVisible();
    fireEvent.change(screen.getByLabelText(/^Current administrator password/), { target: { value: "review-only password" } });
    expect(screen.getByRole("button", { name: "Set up HTTPS and save" })).toBeDisabled();
    fireEvent.click(screen.getByRole("checkbox", { name: /I agree/ }));
    fireEvent.click(screen.getByRole("button", { name: "Set up HTTPS and save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Public TCP 443 did not reach this computer.");
    expect(api.prepareNetworkCertificate).toHaveBeenCalledWith(expect.objectContaining({ listen_address: "192.168.1.10:49717", public_url: "https://8.8.8.8", certificate_mode: "automatic_public", accepted_terms: "https://letsencrypt.org/documents/test-terms.pdf" }), "review-only password", expect.any(AbortSignal));
    expect(api.saveNetwork).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/^Current administrator password/)).toHaveValue("");
  });

  it("creates LAN HTTPS without external discovery and offers the public trust download", async () => {
    vi.mocked(api.networkStatus).mockResolvedValue({ ...status, interfaces: [{ name: "Ethernet", address: "192.168.1.10", loopback: false, private: true }] });
    vi.mocked(api.prepareNetworkCertificate).mockImplementation(async (config) => ({ state: "ready", config }));
    render(<NetworkSettingsPanel backendOnline administrator />);
    fireEvent.click(await screen.findByRole("radio", { name: /LAN \+ local/ }));
    expect(screen.getByText(/Allow incoming TCP 49717.*Router port forwarding is not needed/)).toBeVisible();
    fireEvent.change(screen.getByLabelText(/^Current administrator password/), { target: { value: "review-only password" } });
    fireEvent.click(screen.getByRole("button", { name: "Set up HTTPS and save" }));
    await waitFor(() => expect(api.saveNetwork).toHaveBeenCalledWith(expect.objectContaining({ scope: "lan", certificate_mode: "local_ca", public_url: "https://192.168.1.10:49717" }), "review-only password"));
    expect(await screen.findByRole("link", { name: "Download local trust certificate" })).toHaveAttribute("href", "/api/network/local-trust");
    expect(api.discoverInternet).not.toHaveBeenCalled();
  });

  it("recovers enabled controls after disconnecting during address detection", async () => {
    vi.mocked(api.discoverInternet).mockImplementation(() => new Promise(() => undefined));
    const view = render(<NetworkSettingsPanel backendOnline administrator />);
    fireEvent.click(await screen.findByRole("radio", { name: /^Public/ }));
    await screen.findByRole("button", { name: "Detecting address..." });
    view.rerender(<NetworkSettingsPanel backendOnline={false} administrator />);
    view.rerender(<NetworkSettingsPanel backendOnline administrator />);
    expect(await screen.findByRole("radio", { name: /Local only/ })).toBeEnabled();
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
