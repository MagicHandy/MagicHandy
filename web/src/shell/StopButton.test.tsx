import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { StopButton } from "./StopButton";
import { api } from "../api/client";

const app = vi.hoisted(() => ({ refresh: vi.fn(), show: vi.fn() }));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ refresh: app.refresh, state: { stop_sequence: 0 } }),
  useToast: () => ({ show: app.show }),
}));
vi.mock("../api/client", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/client")>();
  return { ...original, api: { stopMotion: vi.fn(async () => ({ stop_sequence: 1 })) } };
});

describe("StopButton", () => {
  beforeEach(() => setLocaleForTest("en", english));

  it("uses the concise label and enlarges only the square stop glyph", () => {
    render(<StopButton />);

    const button = screen.getByRole("button", { name: "Emergency stop all motion" });
    expect(within(button).getByText("Stop")).toHaveClass("stop-button-label");
    expect(button).not.toHaveTextContent("Stop everything");
    expect(button.querySelector("svg")).toHaveAttribute("width", "21");
    expect(button.querySelector("svg")).toHaveAttribute("height", "21");
  });

  it("keeps the one global Stop inside fullscreen and restores it on exit", async () => {
    const root = document.createElement("div");
    document.body.append(root);
    let fullscreen: Element | null = null;
    const previous = Object.getOwnPropertyDescriptor(document, "fullscreenElement");
    Object.defineProperty(document, "fullscreenElement", { configurable: true, get: () => fullscreen });
    try {
      const view = render(<StopButton className="nav-stop" />);
      fullscreen = root;
      fireEvent(document, new Event("fullscreenchange"));
      const button = screen.getByRole("button", { name: "Emergency stop all motion" });
      expect(root).toContainElement(button);
      expect(screen.getAllByRole("button", { name: "Emergency stop all motion" })).toHaveLength(1);
      fireEvent.click(button);
      await waitFor(() => expect(api.stopMotion).toHaveBeenCalledTimes(1));
      fullscreen = null;
      fireEvent(document, new Event("fullscreenchange"));
      expect(view.container).toContainElement(screen.getByRole("button", { name: "Emergency stop all motion" }));
    } finally {
      if (previous) Object.defineProperty(document, "fullscreenElement", previous);
      else Reflect.deleteProperty(document, "fullscreenElement");
      root.remove();
    }
  });
});
