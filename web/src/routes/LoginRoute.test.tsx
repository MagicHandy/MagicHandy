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
    fireEvent.change(password, { target: { value: "too short" } });
    fireEvent.change(confirmation, { target: { value: "too short" } });
    fireEvent.click(screen.getByRole("button", { name: "Create administrator" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at least 12 bytes");
    expect(auth.bootstrap).not.toHaveBeenCalled();

    fireEvent.change(password, { target: { value: "correct horse battery staple" } });
    fireEvent.change(confirmation, { target: { value: "correct horse battery staple" } });
    fireEvent.click(screen.getByRole("button", { name: "Create administrator" }));
    await waitFor(() => expect(auth.bootstrap).toHaveBeenCalledWith("owner", "correct horse battery staple"));
  });

  it("requires first-account setup on the host computer", () => {
    auth.status = status(false, false);
    render(<LoginRoute />);

    expect(screen.getByRole("alert")).toHaveTextContent("Local setup required");
    expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
  });
});
