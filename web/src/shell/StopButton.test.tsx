import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { StopButton } from "./StopButton";

const app = vi.hoisted(() => ({ refresh: vi.fn(), show: vi.fn() }));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ refresh: app.refresh, state: { stop_sequence: 0 } }),
  useToast: () => ({ show: app.show }),
}));

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
});
