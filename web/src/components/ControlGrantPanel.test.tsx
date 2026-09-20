import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { ControlGrant } from "../api/types";
import { ControlGrantPanel } from "./ControlGrantPanel";

vi.mock("../api/client", () => ({ api: { controlGrant: vi.fn(), grantControl: vi.fn(), revokeControl: vi.fn() } }));
const grant: ControlGrant = { id: "grant", account_id: "operator", issued_by: "owner", created_at: "2026-09-19T00:00:00Z", expires_at: null };
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.controlGrant).mockResolvedValue({ grant: null });
  vi.mocked(api.grantControl).mockResolvedValue({ grant });
  vi.mocked(api.revokeControl).mockResolvedValue({ grant: null });
});

describe("control permission duration", () => {
  it("defaults to a timed grant and saves permanent only after an explicit selection", async () => {
    render(<ControlGrantPanel accountID="operator" disabled={false} />);
    const submit = await screen.findByRole("button", { name: "Grant control permission" });
    await waitFor(() => expect(submit).toBeEnabled());
    expect(screen.getByLabelText("Control permission duration")).toHaveValue("60");
    fireEvent.change(screen.getByLabelText("Control permission duration"), { target: { value: "permanent" } });
    expect(api.grantControl).not.toHaveBeenCalled();
    fireEvent.click(submit);
    await screen.findByText(/Permanent control permission\. It remains active until revoked or replaced\./);
    expect(api.grantControl).toHaveBeenCalledWith("operator", "permanent");
    fireEvent.click(screen.getByRole("button", { name: "Revoke control permission" }));
    await screen.findByText(/Observer access\./);
    expect(api.revokeControl).toHaveBeenCalledWith("operator");
  });

  it("loads a permanent grant accurately and allows replacing it with a timed grant", async () => {
    vi.mocked(api.controlGrant).mockResolvedValueOnce({ grant });
    vi.mocked(api.grantControl).mockResolvedValueOnce({ grant: { ...grant, id: "replacement", expires_at: "2026-09-19T01:00:00Z" } });
    render(<ControlGrantPanel accountID="operator" disabled={false} />);
    await screen.findByText(/Permanent control permission\./);
    expect(screen.getByLabelText("Control permission duration")).toHaveValue("permanent");
    fireEvent.change(screen.getByLabelText("Control permission duration"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Replace control permission" }));
    await screen.findByText(/Control permission expires/);
    expect(api.grantControl).toHaveBeenCalledWith("operator", 60);
  });
});
