import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { AccountRecoveryForm } from "./AccountRecoveryForm";

vi.mock("../api/client", () => ({ api: { recoverPassword: vi.fn() } }));
const recover = vi.mocked(api.recoverPassword);
const savedCode = "ABCD-ABCD-ABCD-ABCD-ABCD-ABCD-ABCD-ABCD";
const fill = (password = "synthetic replacement password", confirmation = password) => {
  fireEvent.change(screen.getByLabelText("Recovery code"), { target: { value: savedCode } });
  fireEvent.change(screen.getByLabelText("New password"), { target: { value: password } });
  fireEvent.change(screen.getByLabelText(/^Confirm password/), { target: { value: confirmation } });
};
const submit = () => fireEvent.submit(screen.getByRole("button", { name: "Reset password with code" }).closest("form")!);
beforeEach(() => { vi.resetAllMocks(); recover.mockResolvedValue({ recovered: true }); });
afterEach(() => vi.useRealTimers());

describe("account recovery sign-in boundary", () => {
  it("validates the replacement password and confirmation before sending credentials", () => {
    render(<AccountRecoveryForm initialUsername="owner" onBack={vi.fn()} />);
    fill("short"); submit();
    expect(screen.getByRole("alert")).toHaveTextContent("at least 8 characters");
    fill("long password", "different password"); submit();
    expect(screen.getByRole("alert")).toHaveTextContent("passwords do not match");
    expect(recover).not.toHaveBeenCalled();
  });

  it("clears credentials and requires a separate sign-in after success", async () => {
    const back = vi.fn();
    render(<AccountRecoveryForm initialUsername=" owner " onBack={back} />);
    fill(); submit();
    expect(await screen.findByRole("status")).toHaveTextContent("Sign in with your new password");
    expect(recover).toHaveBeenCalledWith("owner", savedCode, "synthetic replacement password", expect.any(AbortSignal));
    expect(screen.queryByLabelText("Recovery code")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("New password")).not.toBeInTheDocument();
    expect(back).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Back to sign in" }));
    expect(back).toHaveBeenCalledWith("owner");
  });

  it("reports an uncertain timed-out result and ignores a late success without replaying", async () => {
    vi.useFakeTimers();
    let resolve!: (result: { recovered: boolean }) => void;
    recover.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    render(<AccountRecoveryForm initialUsername="owner" onBack={vi.fn()} />);
    fill(); submit();
    await act(async () => { await vi.advanceTimersByTimeAsync(10_001); });
    expect(recover.mock.calls[0][3]?.aborted).toBe(true);
    expect(screen.getByRole("alert")).toHaveTextContent("Try signing in with the new password before retrying");
    expect(screen.getByRole("button", { name: "Reset password with code" })).toBeEnabled();
    await act(async () => resolve({ recovered: true }));
    expect(screen.queryByText(/Password reset\. Sign in/)).not.toBeInTheDocument();
    expect(recover).toHaveBeenCalledOnce();
  });

  it("aborts requests and removes the deadline on leaving the form", () => {
    vi.useFakeTimers();
    recover.mockImplementationOnce(() => new Promise(() => undefined));
    const view = render(<AccountRecoveryForm initialUsername="owner" onBack={vi.fn()} />);
    fill(); submit(); view.unmount();
    expect(recover.mock.calls[0][3]?.aborted).toBe(true);
    expect(vi.getTimerCount()).toBe(0);
  });
});
