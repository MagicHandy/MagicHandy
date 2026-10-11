import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { defaultDiagnosticReportOptions } from "../api/diagnostic-report-types";
import { DiagnosticReportDownload } from "./DiagnosticReportDownload";

vi.mock("../api/client", () => ({ api: { exportDiagnosticReport: vi.fn() } }));

beforeEach(() => {
  vi.mocked(api.exportDiagnosticReport).mockReset().mockResolvedValue({ blob: new Blob(["zip"]), filename: "diagnostics.zip" });
  vi.stubGlobal("URL", { createObjectURL: vi.fn(() => "blob:report"), revokeObjectURL: vi.fn() });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
});

describe("diagnostic download", () => {
  it("downloads defaults on demand and preserves choices when customization is closed", async () => {
    render(<DiagnosticReportDownload online />);
    expect(api.exportDiagnosticReport).not.toHaveBeenCalled();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download diagnostic report" }));
    await waitFor(() => expect(api.exportDiagnosticReport).toHaveBeenCalledWith(defaultDiagnosticReportOptions, expect.any(AbortSignal)));
    await waitFor(() => expect(screen.getByRole("button", { name: "Download diagnostic report" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Customize report" }));
    expect(screen.getByRole("button", { name: "Customize report" })).toHaveAttribute("aria-expanded", "true");
    fireEvent.change(screen.getByRole("combobox", { name: "Chat content" }), { target: { value: "full" } });
    for (const name of ["Failed model output", "Personas and lore", "Prompts", "Saved memories"]) {
      fireEvent.click(screen.getByRole("switch", { name }));
    }
    fireEvent.click(screen.getByRole("switch", { name: "Motion traces" }));
    fireEvent.click(screen.getByRole("button", { name: "Customize report" }));
    expect(screen.getByText(/Includes:.*Full chat history/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download diagnostic report" }));
    await waitFor(() => expect(api.exportDiagnosticReport).toHaveBeenLastCalledWith({ ...defaultDiagnosticReportOptions, chat: "full", traces: false, model_output: true, personas: true, prompts: true, memories: true }, expect.any(AbortSignal)));
  });

  it("disables output when chat is excluded, handles failures and prevents concurrent downloads", async () => {
    let reject!: (error: Error) => void;
    vi.mocked(api.exportDiagnosticReport).mockImplementation(() => new Promise((_resolve, fail) => { reject = fail; }));
    render(<DiagnosticReportDownload online />);
    fireEvent.click(screen.getByRole("button", { name: "Customize report" }));
    fireEvent.click(screen.getByRole("switch", { name: "Failed model output" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Chat content" }), { target: { value: "none" } });
    expect(screen.getByRole("switch", { name: "Failed model output" })).toBeDisabled();
    expect(screen.getByRole("switch", { name: "Failed model output" })).not.toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Download diagnostic report" }));
    expect(screen.getByRole("button", { name: "Preparing report…" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Preparing report…" }));
    expect(api.exportDiagnosticReport).toHaveBeenCalledTimes(1);
    reject(new Error("diagnostic report could not be completed; retry or exclude optional content"));
    expect(await screen.findByRole("alert")).toHaveTextContent("The report could not be completed.");
    expect(screen.getByRole("button", { name: "Download diagnostic report" })).toBeEnabled();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });

  it("does not download while offline and aborts on navigation", () => {
    const view = render(<DiagnosticReportDownload online={false} />);
    expect(screen.getByRole("button", { name: "Download diagnostic report" })).toBeDisabled();
    view.rerender(<DiagnosticReportDownload online />);
    vi.mocked(api.exportDiagnosticReport).mockImplementation(() => new Promise(() => {}));
    fireEvent.click(screen.getByRole("button", { name: "Download diagnostic report" }));
    const signal = vi.mocked(api.exportDiagnosticReport).mock.calls[0][1]!;
    view.unmount();
    expect(signal.aborted).toBe(true);
  });
});
