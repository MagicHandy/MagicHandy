import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AUTHENTICATION_REQUIRED_EVENT } from "../api/client";
import type { AuthenticationStatus, ControlIdentity, UserAccount } from "../api/types";
import { AuthProvider, useAuth } from "./auth";

const owner: UserAccount = {
  id: "owner-id",
  username: "owner",
  role: "admin",
  disabled: false,
  has_profile_image: false,
  created_at: "2026-08-28T12:00:00Z",
  updated_at: "2026-08-28T12:00:00Z",
};

const self: ControlIdentity = {
  account: owner,
  relationship: "self",
  label: "Self",
  selected: true,
};

function authenticationStatus(overrides: Partial<AuthenticationStatus> = {}): AuthenticationStatus {
  return {
    initialized: true,
    authentication_required: true,
    authenticated: true,
    bootstrap_available: true,
    ui_locale: "en",
    account: owner,
    control_identities: [self],
    ...overrides,
  };
}

function response(data: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => data === null ? "" : JSON.stringify(data),
  } as Response;
}

function Probe() {
  const auth = useAuth();
  return (
    <div>
      <span>{auth.loading ? "loading" : auth.status?.authenticated ? "authenticated" : "locked"}</span>
      <span>{auth.status?.control_identities?.find((identity) => identity.selected)?.label ?? "none"}</span>
      <button onClick={() => void auth.login("owner", "correct horse battery staple")}>login</button>
      <button onClick={() => void auth.selectControlIdentity("linked-id")}>select linked</button>
      <button onClick={() => void auth.logout().catch(() => undefined)}>logout</button>
    </div>
  );
}

describe("AuthProvider", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("keeps session cookies backend-owned and refreshes status after login", async () => {
    let signedIn = false;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/auth/login") {
        signedIn = true;
        return response({ account: owner, expires_at: "2026-08-29T00:00:00Z" });
      }
      if (path === "/api/auth/status") {
        return response(authenticationStatus(signedIn ? {} : { authenticated: false, account: null, control_identities: null }));
      }
      throw new Error(`unexpected request ${path}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("locked")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "login" }));
    expect(await screen.findByText("authenticated")).toBeInTheDocument();

    const loginCall = fetchMock.mock.calls.find(([path]) => String(path) === "/api/auth/login");
    expect(loginCall?.[1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(String(loginCall?.[1]?.body))).toEqual({ username: "owner", password: "correct horse battery staple" });
  });

  it("updates the selected linked identity only for the current session view", async () => {
    const linked: ControlIdentity = {
      account: { ...owner, id: "linked-id", username: "partner", role: "operator" },
      relationship: "linked",
      label: "Partner",
      selected: true,
    };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/auth/status") return response(authenticationStatus());
      if (String(input) === "/api/auth/control-identity") return response({ control_identities: [{ ...self, selected: false }, linked] });
      throw new Error(`unexpected request ${String(input)}`);
    }));

    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("Self")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "select linked" }));
    expect(await screen.findByText("Partner")).toBeInTheDocument();
  });

  it("reacts to an API 401 by returning to the login boundary", async () => {
    let expired = false;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) !== "/api/auth/status") throw new Error(`unexpected request ${String(input)}`);
      return response(expired
        ? authenticationStatus({ authenticated: false, account: null, control_identities: null })
        : authenticationStatus());
    }));

    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("authenticated")).toBeInTheDocument();
    expired = true;
    act(() => window.dispatchEvent(new Event(AUTHENTICATION_REQUIRED_EVENT)));
    await waitFor(() => expect(screen.getByText("locked")).toBeInTheDocument());
  });

  it("refreshes the login boundary when explicit logout races an expired session", async () => {
    let expired = false;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/api/auth/logout") {
        expired = true;
        return response({ error: "authentication required" }, 401);
      }
      if (path === "/api/auth/status") {
        return response(expired
          ? authenticationStatus({ authenticated: false, account: null, control_identities: null })
          : authenticationStatus());
      }
      throw new Error(`unexpected request ${path}`);
    }));

    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("authenticated")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "logout" }));
    expect(await screen.findByText("locked")).toBeInTheDocument();
  });
});
