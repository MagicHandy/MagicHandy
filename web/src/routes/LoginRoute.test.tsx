import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setLocaleForTest } from "../i18n";
import { LoginRoute } from "./LoginRoute";

const auth = vi.hoisted(() => ({
  status: null as Record<string, unknown> | null,
  login: vi.fn(async () => undefined),
  bootstrap: vi.fn(async () => undefined),
}));

vi.mock("../state/auth", () => ({
  useAuth: () => auth,
}));

function status(initialized: boolean, bootstrapAvailable = true) {
  return {
    initialized,
    authentication_required: true,
    authenticated: false,
    bootstrap_available: bootstrapAvailable,
    ui_locale: "en",
    account: null,
    control_identities: null,
  };
}

describe("LoginRoute", () => {
  beforeEach(() => {
    setLocaleForTest("en");
    auth.status = status(true);
    auth.login.mockClear();
    auth.bootstrap.mockClear();
  });

  it("uses the JSON login boundary without exposing session tokens", async () => {
    render(<LoginRoute />);

    fireEvent.change(screen.getByLabelText("Username"), { target: { value: " owner " } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "correct horse battery staple" } });
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await waitFor(() => expect(auth.login).toHaveBeenCalledWith("owner", "correct horse battery staple"));
    expect(screen.getByText("Emergency Stop remains available.")).toBeInTheDocument();
  });

  it("validates and creates the initial local administrator", async () => {
    auth.status = status(false);
    render(<LoginRoute />);

    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "owner" } });
    const password = screen.getByText("Password", { selector: ".label" }).closest("label")!.querySelector("input")!;
    const confirmation = screen.getByText("Confirm password", { selector: ".label" }).closest("label")!.querySelector("input")!;
    fireEvent.change(password, { target: { value: "short77" } });
    fireEvent.change(confirmation, { target: { value: "short77" } });
    expect(screen.getByText("Passwords match.")).toHaveAttribute("data-state", "match");
    fireEvent.click(screen.getByRole("button", { name: "Create administrator" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at least 8 characters");
    expect(auth.bootstrap).not.toHaveBeenCalled();

    fireEvent.change(password, { target: { value: "eight888" } });
    fireEvent.change(confirmation, { target: { value: "eight889" } });
    expect(confirmation).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("The passwords do not match.")).toHaveAttribute("data-state", "mismatch");

    fireEvent.change(confirmation, { target: { value: "eight888" } });
    expect(confirmation).toHaveAttribute("aria-invalid", "false");
    expect(screen.getByText("Passwords match.")).toHaveAttribute("data-state", "match");
    fireEvent.click(screen.getByRole("button", { name: "Create administrator" }));
    await waitFor(() => expect(auth.bootstrap).toHaveBeenCalledWith("owner", "eight888"));
  });

  it("requires first-account setup on the host computer", () => {
    auth.status = status(false, false);
    render(<LoginRoute />);

    expect(screen.getByRole("alert")).toHaveTextContent("Local setup required");
    expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Forgot password?" })).not.toBeInTheDocument();
  });

  it("keeps recovery separate from signing in and discards the typed login password", () => {
    render(<LoginRoute />);
    expect(screen.queryByLabelText("Recovery code")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "owner" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "unsubmitted password" } });
    fireEvent.click(screen.getByRole("button", { name: "Forgot password?" }));
    expect(screen.getByRole("heading", { name: "Recover your account" })).toBeInTheDocument();
    expect(screen.getByLabelText("Recovery code")).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("owner");
    expect(screen.getByText("Emergency Stop remains available.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to sign in" }));
    expect(screen.getByLabelText("Password")).toHaveValue("");
    expect(screen.queryByLabelText("Recovery code")).not.toBeInTheDocument();
    expect(auth.login).not.toHaveBeenCalled();
  });
});
