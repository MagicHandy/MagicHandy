import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ManualMotionTest } from "./ManualMotionTest";
import { WorkspaceHead } from "./WorkspaceHead";
import { NavRail, routeBase } from "../shell/NavRail";

const app = vi.hoisted(() => ({
  route: "#/chat",
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: { startManualTest: vi.fn(), stopMotion: vi.fn() },
}));

vi.mock("../state/app-state", () => ({
  useHashRoute: () => app.route,
  useAppState: () => ({
    state: { settings: { device: { hsp_dispatch_owner: "cloud_rest" } } },
    backendOnline: true,
    readOnly: false,
    motion: { engine: { running: false } },
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

vi.mock("../shell/StopButton", () => ({ StopButton: () => <button type="button">Emergency Stop</button> }));

describe("shell accessibility", () => {
  it("keeps every compact navigation link named and normalizes unknown routes", () => {
    render(<NavRail />);

    expect(screen.getByRole("link", { name: "Chat" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Preset modes" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Pattern library" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings" })).toBeInTheDocument();
    expect(routeBase("#/not-a-route/details")).toBe("chat");
  });

  it("names the manual speed slider explicitly", () => {
    render(<ManualMotionTest />);
    expect(screen.getByRole("slider", { name: "Speed" })).toHaveValue("50");
  });

  it("updates the document title and focuses the route heading", () => {
    render(<WorkspaceHead title="Pattern library" />);
    expect(document.title).toBe("Pattern library | MagicHandy");
    expect(screen.getByRole("heading", { level: 1, name: "Pattern library" })).toHaveFocus();
  });
});
