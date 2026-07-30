import { fireEvent, render, screen } from "@testing-library/react";
import { ToastProvider, useNotifications, useToast } from "./app-state";

beforeEach(() => {
  window.sessionStorage.clear();
});

function NotificationHarness() {
  const { show } = useToast();
  const { items, unreadCount, push, markAllRead, clear } = useNotifications();

  return (
    <>
      <button type="button" onClick={() => show("Library scan failed", "error")}>Show toast</button>
      <button
        type="button"
        onClick={() => push({
          title: "Library scan complete",
          tone: "success",
          category: "library",
          sourceKey: "scan-complete:one",
        })}
      >
        Push scan result
      </button>
      <button
        type="button"
        onClick={() => push({
          title: "New library scan complete",
          tone: "success",
          category: "library",
          sourceKey: "scan-complete:two",
        })}
      >
        Push newer scan result
      </button>
      <button type="button" onClick={markAllRead}>Mark read</button>
      <button type="button" onClick={clear}>Clear</button>
      <output aria-label="Unread count">{unreadCount}</output>
      <ul aria-label="Notification history">
        {items.map((item) => <li key={item.id}>{item.title}</li>)}
      </ul>
    </>
  );
}

describe("notification feedback channel", () => {
  it("retains toast feedback in notification history", () => {
    render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show toast" }));

    expect(screen.getByText("Library scan failed", { selector: ".toast" })).toBeVisible();
    expect(screen.getByRole("list", { name: "Notification history" })).toHaveTextContent("Library scan failed");
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("1");
  });

  it("deduplicates backend events by source key and manages unread state", () => {
    render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );

    const push = screen.getByRole("button", { name: "Push scan result" });
    fireEvent.click(push);
    fireEvent.click(push);

    expect(screen.getAllByText("Library scan complete")).toHaveLength(1);
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("1");

    fireEvent.click(screen.getByRole("button", { name: "Mark read" }));
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("0");

    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.getByRole("list", { name: "Notification history" })).toBeEmptyDOMElement();

    fireEvent.click(push);
    expect(screen.getByRole("list", { name: "Notification history" })).toBeEmptyDOMElement();
  });

  it("keeps cleared backend events dismissed across provider remounts", () => {
    const first = render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Push scan result" }));
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    first.unmount();

    render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Push scan result" }));

    expect(screen.getByRole("list", { name: "Notification history" })).toBeEmptyDOMElement();
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("0");

    fireEvent.click(screen.getByRole("button", { name: "Push newer scan result" }));
    expect(screen.getByRole("list", { name: "Notification history" })).toHaveTextContent("New library scan complete");
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("1");
  });

  it("restores current-session history and read state after a remount", () => {
    const first = render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Push scan result" }));
    fireEvent.click(screen.getByRole("button", { name: "Mark read" }));
    first.unmount();

    render(
      <ToastProvider>
        <NotificationHarness />
      </ToastProvider>,
    );

    expect(screen.getByRole("list", { name: "Notification history" })).toHaveTextContent("Library scan complete");
    expect(screen.getByRole("status", { name: "Unread count" })).toHaveTextContent("0");
  });
});
