import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

const app = vi.hoisted(() => ({
  route: "#/settings/device",
  refresh: vi.fn(),
  state: {} as Record<string, unknown>,
}));

vi.mock("./state/app-state", () => ({
  useHashRoute: () => app.route,
  useAppState: () => ({ state: app.state, startupError: "", refresh: app.refresh, readOnly: false }),
}));

vi.mock("./shell/AppShell", () => ({
  AppShell: ({ children }: { children: ReactNode }) => <main id="workspace">{children}</main>,
}));

vi.mock("./routes/SettingsRoute", async () => {
  const React = await import("react");
  return {
    SettingsRoute: () => {
      const [draft, setDraft] = React.useState("");
      return <input aria-label="Settings draft" value={draft} onChange={(event) => setDraft(event.target.value)} />;
    },
  };
});

vi.mock("./routes/ChatRoute", () => ({ ChatRoute: () => <div>Chat route</div> }));
vi.mock("./routes/PresetModesRoute", () => ({ PresetModesRoute: () => <div>Modes route</div> }));
vi.mock("./routes/PatternLibraryRoute", () => ({ PatternLibraryRoute: () => <div>Library route</div> }));
vi.mock("./routes/VideoRoute", () => ({ VideoRoute: () => <div>Videos route</div> }));
vi.mock("./routes/SetupRoute", () => ({ SetupRoute: () => <div>Setup route</div> }));

const client = vi.hoisted(() => ({ completeSetup: vi.fn().mockResolvedValue({ settings: {} }), llmDuplicates: vi.fn().mockRejectedValue(new Error("no")) }));
vi.mock("./api/client", () => ({ api: client }));

describe("App route lifetime", () => {
  beforeEach(() => {
    client.completeSetup.mockClear();
    app.state = {};
    window.location.hash = "#/chat";
  });

  it("redirects a fresh data store into guided setup", async () => {
    app.route = "#/chat";
    // using_defaults marks a store with no saved settings document at all, which
    // is the only case that should take over the route.
    app.state = { settings: { ui: { setup_completed: false } }, settings_status: { using_defaults: true } };
    render(<App />);

    await waitFor(() => expect(window.location.hash).toBe("#/setup"));
  });

  // A store that already holds settings but is not marked configured is an
  // update or an abandoned setup, not a first run. Taking over the route there
  // gave the user no way to decline and repeated on every launch.
  it("asks instead of redirecting when a configured store is not marked complete", async () => {
    app.route = "#/chat";
    app.state = { settings: { ui: { setup_completed: false } }, settings_status: { using_defaults: false } };
    render(<App />);

    expect(await screen.findByRole("dialog", { name: "Run setup again?" })).toBeInTheDocument();
    expect(window.location.hash).toBe("#/chat");
    expect(screen.getByText("Chat route")).toBeInTheDocument();
  });

  it("declining records the choice so the question does not return next launch", async () => {
    app.route = "#/chat";
    app.state = { settings: { ui: { setup_completed: false } }, settings_status: { using_defaults: false } };
    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: "Not now" }));

    await waitFor(() => expect(client.completeSetup).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(window.location.hash).toBe("#/chat");
  });

  it("accepting opens setup", async () => {
    app.route = "#/chat";
    app.state = { settings: { ui: { setup_completed: false } }, settings_status: { using_defaults: false } };
    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: "Run setup" }));

    await waitFor(() => expect(window.location.hash).toBe("#/setup"));
  });

  it("preserves settings drafts between subsections but resets them after leaving Settings", () => {
    app.route = "#/settings/device";
    const result = render(<App />);
    fireEvent.change(screen.getByRole("textbox", { name: "Settings draft" }), { target: { value: "49999" } });

    app.route = "#/settings/model";
    result.rerender(<App />);
    expect(screen.getByRole("textbox", { name: "Settings draft" })).toHaveValue("49999");

    app.route = "#/chat";
    result.rerender(<App />);
    expect(screen.getByText("Chat route")).toBeInTheDocument();

    app.route = "#/settings/device";
    result.rerender(<App />);
    expect(screen.getByRole("textbox", { name: "Settings draft" })).toHaveValue("");
  });

  it("routes Videos independently from the pattern library", () => {
    app.route = "#/videos";
    render(<App />);

    expect(screen.getByText("Videos route")).toBeInTheDocument();
    expect(screen.queryByText("Library route")).not.toBeInTheDocument();
  });
});
