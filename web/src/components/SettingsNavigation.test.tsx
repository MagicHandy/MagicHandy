import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsNavigation } from "./SettingsNavigation";

let available = 300, content = 700;
let resized: () => void;
const disconnect = vi.fn();
function view(current = "first") {
  return <SettingsNavigation current={current} className="settings-nav" label="Settings sections">
    <a href="#/first" data-position="0" aria-current={current === "first" ? "page" : undefined}>First</a>
    <a href="#/last" data-position="620" aria-current={current === "last" ? "page" : undefined}>Last</a>
  </SettingsNavigation>;
}
beforeEach(() => {
  available = 300; content = 700; disconnect.mockClear();
  vi.stubGlobal("ResizeObserver", class {
    constructor(callback: () => void) { resized = callback; }
    observe() {}
    disconnect = disconnect;
  });
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
    return this.classList.contains("settings-navigation") ? available : available - 56;
  });
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(content);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const strip = this.closest(".settings-nav-scrollport") as HTMLElement;
    const left = this.hasAttribute("data-position") ? Number(this.dataset.position) - strip.scrollLeft : 0;
    const width = this.hasAttribute("data-position") ? 60 : this.clientWidth;
    return { left, right: left + width, width, top: 0, bottom: 36, height: 36, x: left, y: 0, toJSON() {} };
  });
});
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("responsive settings navigation", () => {
  it("reveals a bookmarked destination and returns to earlier links without scrolling the page", () => {
    const rendered = render(view());
    const strip = rendered.container.querySelector<HTMLElement>(".settings-nav-scrollport")!;
    expect(strip.scrollLeft).toBe(0);
    rendered.rerender(view("last"));
    const last = screen.getByRole("link", { name: "Last" }).getBoundingClientRect();
    expect(last.left).toBeGreaterThanOrEqual(0);
    expect(last.right).toBeLessThanOrEqual(strip.clientWidth);
    expect(strip.scrollTop).toBe(0);
    rendered.rerender(view());
    expect(strip.scrollLeft).toBe(0);
  });

  it("scrolls bounded overflow without selecting another destination", () => {
    const rendered = render(view());
    const strip = rendered.container.querySelector<HTMLElement>(".settings-nav-scrollport")!;
    const previous = screen.getByRole("button", { name: "Scroll sections left" });
    const next = screen.getByRole("button", { name: "Scroll sections right" });
    expect(previous).toBeDisabled();
    fireEvent.click(next); fireEvent.click(next); fireEvent.click(next);
    expect(strip.scrollLeft).toBe(content - strip.clientWidth);
    expect(next).toBeDisabled();
    expect(screen.getByRole("link", { name: "First" })).toHaveAttribute("aria-current", "page");
    fireEvent.click(previous);
    expect(strip.scrollLeft).toBeLessThan(content - strip.clientWidth);
    expect(next).toBeEnabled();
  });

  it("removes overflow controls when resizing makes the links fit and releases its observer", () => {
    const rendered = render(view());
    expect(screen.getByRole("button", { name: "Scroll sections right" })).toBeVisible();
    available = 800;
    act(() => resized());
    expect(screen.queryByRole("button", { name: "Scroll sections right" })).not.toBeInTheDocument();
    rendered.unmount();
    expect(disconnect).toHaveBeenCalledOnce();
  });
});
