import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ChatTabs } from "./ChatTabs";

const sessions = [
  { id: "one", title: "Working draft", saved: false, active: true, message_count: 2, latest_seq: 2, created_at: "now", updated_at: "now" },
  { id: "two", title: "Saved conversation", saved: true, active: false, message_count: 4, latest_seq: 6, created_at: "now", updated_at: "now" },
];

describe("ChatTabs", () => {
  it("supports ordinary activation and a right-click save action", async () => {
    const activate = vi.fn();
    const save = vi.fn();
    const start = vi.fn();
    render(
      <ChatTabs
        sessions={sessions}
        activeId="one"
        disabled={false}
        onActivate={activate}
        onNew={start}
        onSave={save}
        onDelete={vi.fn()}
      />,
    );

    const active = screen.getByRole("tab", { name: /Working draft/ });
    expect(active).toHaveAttribute("aria-selected", "true");
    fireEvent.contextMenu(active, { clientX: 40, clientY: 30 });
    const saveItem = screen.getByRole("menuitem", { name: "Save chat" });
    await waitFor(() => expect(saveItem).toHaveFocus());
    fireEvent.click(saveItem);
    expect(save).toHaveBeenCalledWith(sessions[0]);

    fireEvent.click(screen.getByRole("tab", { name: "Saved conversation" }));
    expect(activate).toHaveBeenCalledWith(sessions[1]);
    fireEvent.click(screen.getByRole("button", { name: "Start a new chat" }));
    expect(start).toHaveBeenCalledOnce();

    const title = screen.getByRole("heading", { name: "Chat", level: 1 });
    const tablist = screen.getByRole("tablist", { name: "Chat sessions" });
    const newButton = screen.getByRole("button", { name: "Start a new chat" });
    expect(title.closest(".chat-tabs-bar")).toContainElement(tablist);
    expect(tablist.closest(".chat-tabs-scroll")?.nextElementSibling).toContainElement(newButton);
    expect(tablist.closest(".chat-tabs-scroll")).toHaveStyle({ width: "482px" });
    expect(newButton).toHaveClass("chat-new-button");
    expect(newButton).not.toHaveClass("icon-button");
  });

  it("does not put a disabled menu hit target beside New Chat", () => {
    render(
      <ChatTabs
        sessions={[{ ...sessions[1], active: true }]}
        activeId="two"
        disabled={false}
        onActivate={vi.fn()}
        onNew={vi.fn()}
        onSave={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Open options for Saved conversation" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start a new chat" })).toBeEnabled();
  });

  it("keeps backend-reported mood outside the Chat heading", () => {
    render(
      <ChatTabs
        sessions={sessions}
        activeId="one"
        disabled={false}
        assistantMood="Teasing"
        onActivate={vi.fn()}
        onNew={vi.fn()}
        onSave={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Chat", level: 1 })).toBeInTheDocument();
    expect(screen.getByRole("status", { name: "Assistant mood: Teasing" })).toHaveTextContent("MoodTeasing");
  });

  it("moves keyboard focus across tabs without activating them", () => {
    const activate = vi.fn();
    render(
      <ChatTabs
        sessions={sessions}
        activeId="one"
        disabled={false}
        onActivate={activate}
        onNew={vi.fn()}
        onSave={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    const first = screen.getByRole("tab", { name: /Working draft/ });
    const second = screen.getByRole("tab", { name: "Saved conversation" });
    first.focus();
    fireEvent.keyDown(first, { key: "ArrowRight" });
    expect(second).toHaveFocus();
    expect(activate).not.toHaveBeenCalled();
  });

  it("uses the wheel to traverse overflow after tabs reach their compact limit", () => {
    render(
      <ChatTabs
        sessions={sessions}
        activeId="one"
        disabled={false}
        onActivate={vi.fn()}
        onNew={vi.fn()}
        onSave={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    const strip = screen.getByRole("tablist", { name: "Chat sessions" }).closest(".chat-tabs-scroll") as HTMLDivElement;
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 300 },
      scrollWidth: { configurable: true, value: 482 },
    });
    const wheel = new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: 48 });
    fireEvent(strip, wheel);
    expect(strip.scrollLeft).toBe(48);
    expect(wheel.defaultPrevented).toBe(true);
  });
});
