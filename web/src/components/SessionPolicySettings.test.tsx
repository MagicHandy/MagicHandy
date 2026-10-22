import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { SessionPolicySettings } from "./SessionPolicySettings";

vi.mock("../api/client", () => ({ api: { sessionPolicy: vi.fn(), saveSessionPolicy: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.sessionPolicy).mockResolvedValue({ idle_minutes: 480, lifetime_minutes: 720 });
});

it("shows the backend policy and applies a confirmed selection without taking control", async () => {
  const changed = vi.fn();
  let finish!: (value: { idle_minutes: number; lifetime_minutes: number }) => void;
  vi.mocked(api.saveSessionPolicy).mockReturnValue(new Promise(resolve => { finish = resolve; }));
  render(<SessionPolicySettings backendOnline onChanged={changed} />);
  const select = screen.getByRole("combobox", { name: "Sign out after inactivity" });
  await waitFor(() => expect(select).toHaveValue("480"));
  fireEvent.change(select, { target: { value: "0" } });
  expect(select).toBeDisabled();
  expect(select).toHaveValue("480");
  expect(api.saveSessionPolicy).toHaveBeenCalledWith(0, expect.any(AbortSignal));
  await act(async () => finish({ idle_minutes: 0, lifetime_minutes: 720 }));
  expect(select).toHaveValue("0");
  expect(changed).toHaveBeenCalledOnce();
  expect(screen.getByText(/Logins still expire 12 hours/)).toBeVisible();
});

it("reports an unconfirmed save and reads back its outcome without replaying it", async () => {
  vi.mocked(api.saveSessionPolicy).mockRejectedValue(new Error("Connection lost"));
  render(<SessionPolicySettings backendOnline onChanged={vi.fn()} />);
  const select = screen.getByRole("combobox");
  await waitFor(() => expect(select).toBeEnabled());
  fireEvent.change(select, { target: { value: "30" } });
  expect(await screen.findByRole("alert")).toHaveTextContent("Could not confirm");
  expect(select).toHaveValue("480");
  vi.mocked(api.sessionPolicy).mockResolvedValue({ idle_minutes: 30, lifetime_minutes: 720 });
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(select).toHaveValue("30"));
  expect(api.saveSessionPolicy).toHaveBeenCalledOnce();
});

it("cancels reads when the settings page closes", async () => {
  let signal: AbortSignal | undefined;
  vi.mocked(api.sessionPolicy).mockImplementation(abort => { signal = abort; return new Promise(() => {}); });
  const view = render(<SessionPolicySettings backendOnline onChanged={vi.fn()} />);
  view.unmount();
  expect(signal?.aborted).toBe(true);
});
