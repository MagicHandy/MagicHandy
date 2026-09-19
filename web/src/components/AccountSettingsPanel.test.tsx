import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { UserAccount } from "../api/types";
import { AccountSettingsPanel } from "./AccountSettingsPanel";

const state = vi.hoisted(() => ({ route: "#/settings/access", role: "admin", session: "first-login", id: "a".repeat(32), refresh: vi.fn(), show: vi.fn() }));
vi.mock("../state/auth", () => ({ useAuth: () => ({ status: { initialized: true, session_id: state.session,
  account: { id: state.id, username: "owner", role: state.role, disabled: false, has_profile_image: false }, control_identities: [] }, refresh: state.refresh }) }));
vi.mock("../state/app-state", () => ({ useHashRoute: () => state.route, useToast: () => ({ show: state.show }) }));
vi.mock("./NetworkSettingsPanel", () => ({ NetworkSettingsPanel: () => <div>Network configuration fixture</div> }));
vi.mock("./SessionSettingsPanel", () => ({ SessionSettingsPanel: () => <div>Own sessions fixture</div> }));
vi.mock("./AuditSettingsPanel", () => ({ AuditSettingsPanel: () => <div>Access history fixture</div> }));
vi.mock("../api/client", () => ({ api: { accounts: vi.fn(), controlGrant: vi.fn(), recoveryCodeStatus: vi.fn(), accountProfileImageURL: () => "" } }));
const accounts = vi.mocked(api.accounts), grant = vi.mocked(api.controlGrant);
const operator = { id: "b".repeat(32), username: "operator", role: "operator", disabled: false, has_profile_image: false } as UserAccount;

beforeEach(() => {
  vi.resetAllMocks(); state.route = "#/settings/access"; state.role = "admin"; state.session = "first-login"; state.id = "a".repeat(32);
  accounts.mockResolvedValue({ accounts: [operator] }); grant.mockResolvedValue({ grant: null });
  vi.mocked(api.recoveryCodeStatus).mockResolvedValue({ remaining: 0, limit: 8 });
});
afterEach(() => vi.useRealTimers());

describe("account settings task navigation", () => {
  it("opens a focused profile without loading administration or unrelated panels", () => {
    render(<AccountSettingsPanel backendOnline />);
    const nav = screen.getByRole("navigation", { name: "Access sections" });
    expect(within(nav).getAllByRole("link")).toHaveLength(6);
    expect(within(nav).getByRole("link", { name: "Your profile" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Remote access" })).toHaveAttribute("href", "#/settings/access/network");
    expect(screen.queryByLabelText("New password")).not.toBeInTheDocument();
    expect(screen.queryByText("Own sessions fixture")).not.toBeInTheDocument();
    expect(screen.queryByText("Network configuration fixture")).not.toBeInTheDocument();
    expect(screen.queryByText(/Future invitation-based/)).not.toBeInTheDocument();
    expect(accounts).not.toHaveBeenCalled(); expect(grant).not.toHaveBeenCalled();
  });

  it.each(["accounts", "network", "history"])("keeps an operator bookmark for %s within personal settings", (section) => {
    state.role = "operator"; state.route = `#/settings/access/${section}`;
    render(<AccountSettingsPanel backendOnline />);
    const nav = screen.getByRole("navigation", { name: "Access sections" });
    expect(within(nav).getAllByRole("link")).toHaveLength(3);
    expect(within(nav).getByRole("link", { name: "Your profile" })).toHaveAttribute("aria-current", "page");
    expect(screen.queryByText("Network configuration fixture")).not.toBeInTheDocument();
    expect(screen.queryByText("Access history fixture")).not.toBeInTheDocument();
    expect(accounts).not.toHaveBeenCalled();
  });

  it("clears a security draft when navigating or changing login", () => {
    state.route = "#/settings/access/security";
    const view = render(<AccountSettingsPanel backendOnline />);
    fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "old private draft" } });
    state.route = "#/settings/access/sessions"; view.rerender(<AccountSettingsPanel backendOnline />);
    expect(screen.getByText("Own sessions fixture")).toBeInTheDocument();
    state.route = "#/settings/access/security"; view.rerender(<AccountSettingsPanel backendOnline />);
    expect(screen.getByLabelText("Current password")).toHaveValue("");
    fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "another draft" } });
    state.session = "replacement-login"; view.rerender(<AccountSettingsPanel backendOnline />);
    expect(screen.getByLabelText("Current password")).toHaveValue("");
    expect(accounts).not.toHaveBeenCalled();
  });

  it("loads administration on demand and expands only the chosen account controls", async () => {
    state.route = "#/settings/access/accounts";
    render(<AccountSettingsPanel backendOnline />);
    await screen.findByText("operator", { selector: "strong" });
    expect(accounts).toHaveBeenCalledWith(expect.any(AbortSignal));
    expect(grant).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Create account" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add an account" }));
    expect(screen.getByRole("button", { name: "Create account" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Control permission" }));
    await screen.findByRole("button", { name: "Grant control permission" });
    expect(grant).toHaveBeenCalledWith(operator.id, expect.any(AbortSignal));
    fireEvent.click(screen.getByRole("button", { name: "Control permission" }));
    expect(grant.mock.calls[0][1]?.aborted).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    fireEvent.change(screen.getByLabelText("New password for operator"), { target: { value: "abandoned reset draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    expect(screen.getByLabelText("New password for operator")).toHaveValue("");
  });

  it("cancels a directory read on permission loss and ignores its late result", async () => {
    let resolve!: (value: { accounts: UserAccount[] }) => void;
    accounts.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    state.route = "#/settings/access/accounts";
    const view = render(<AccountSettingsPanel backendOnline />);
    expect(accounts).toHaveBeenCalledOnce();
    state.role = "operator"; view.rerender(<AccountSettingsPanel backendOnline />);
    expect(accounts.mock.calls[0][0]?.aborted).toBe(true);
    await act(async () => resolve({ accounts: [operator] }));
    expect(screen.queryByRole("heading", { name: "Installation accounts" })).not.toBeInTheDocument();
    expect(screen.queryByText("operator", { selector: "strong" })).not.toBeInTheDocument();
  });

  it("does not offer a control grant when the current permission could not be read", async () => {
    grant.mockRejectedValueOnce(new Error("permission read failed"));
    state.route = "#/settings/access/accounts";
    render(<AccountSettingsPanel backendOnline />);
    await screen.findByText("operator", { selector: "strong" });
    fireEvent.click(screen.getByRole("button", { name: "Control permission" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("permission read failed");
    expect(screen.getByRole("button", { name: "Grant control permission" })).toBeDisabled();
    expect(screen.queryByText("Observer access. This account can view shared content and use Stop.")).not.toBeInTheDocument();
  });

  it("bounds a stalled directory read and recovers through explicit refresh", async () => {
    vi.useFakeTimers(); accounts.mockImplementationOnce(() => new Promise(() => undefined));
    state.route = "#/settings/access/accounts";
    const view = render(<AccountSettingsPanel backendOnline />);
    await act(async () => { await vi.advanceTimersByTimeAsync(10_001); });
    expect(screen.getByRole("alert")).toHaveTextContent("Accounts are unavailable");
    expect(accounts.mock.calls[0][0]?.aborted).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Refresh accounts" }));
    await act(async () => undefined);
    expect(screen.getByText("operator", { selector: "strong" })).toBeInTheDocument();
    view.unmount(); expect(vi.getTimerCount()).toBe(0);
  });
});
