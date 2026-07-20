import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";

const app = vi.hoisted(() => ({
  route: "#/settings/device",
  refresh: vi.fn(),
}));

vi.mock("./state/app-state", () => ({
  useHashRoute: () => app.route,
  useAppState: () => ({ state: {}, startupError: "", refresh: app.refresh }),
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

describe("App route lifetime", () => {
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
