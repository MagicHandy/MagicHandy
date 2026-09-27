import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "../api/client";
import type { RemoteState } from "../api/remote-types";
import type { MediaVideo } from "../api/types";
import { RemoteRoute } from "./RemoteRoute";

const app = vi.hoisted(() => ({
  value: { backendOnline: true, readOnly: true, state: { capabilities: { control: true }, chat: { latest_seq: 4 } } as Record<string, unknown> },
}));

vi.mock("../state/app-state", () => ({ useAppState: () => app.value }));
vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    api: {
      remoteState: vi.fn(),
      remoteEventsURL: () => "/api/remote/events?client_id=phone",
      sendRemoteCommand: vi.fn(),
      mediaVideos: vi.fn(),
      getChatMessages: vi.fn(),
    },
  };
});

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  readonly listeners = new Map<string, Set<(event: MessageEvent) => void>>();
  onerror: (() => void) | null = null;
  closed = false;
  constructor(readonly url: string) {
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(listener);
  }
  close() {
    this.closed = true;
  }
  emit(type: string, data: unknown) {
    for (const listener of this.listeners.get(type) ?? []) listener(new MessageEvent(type, { data: JSON.stringify(data) }));
  }
}

const catalog: MediaVideo[] = [
  { id: "take-7", location_path: "C:/media", display_name: "take07.mp4", title: "Evening take", size_bytes: 1, modified_at: "2026-09-01T00:00:00Z", duration_ms: 90_000, has_funscript: true, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: ["calm"] },
  { id: "other", location_path: "C:/media", display_name: "other.mp4", size_bytes: 1, modified_at: "2026-09-01T00:00:00Z", duration_ms: 30_000, has_funscript: false, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: [] },
];

function desktop(patch: Partial<RemoteState> = {}): RemoteState {
  return {
    revision: 3, connected: true, route: "videos", pending: 0, recent: [],
    video: { video_id: "take-7", title: "Evening take", playing: false, position_ms: 12_000, duration_ms: 90_000, volume: 0.8, muted: false, rate: 1, synchronized: true, sync_state: "idle", ready: true },
    ...patch,
  };
}

function publish(state: RemoteState) {
  act(() => FakeEventSource.instances[FakeEventSource.instances.length - 1].emit("state", state));
}

describe("phone remote", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
    FakeEventSource.instances = [];
    vi.stubGlobal("EventSource", FakeEventSource);
    app.value = { backendOnline: true, readOnly: true, state: { capabilities: { control: true }, chat: { latest_seq: 4 } } };
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: catalog } as Awaited<ReturnType<typeof api.mediaVideos>>);
    vi.mocked(api.remoteState).mockResolvedValue({ remote: desktop({ connected: false, video: undefined }) });
    vi.mocked(api.sendRemoteCommand).mockImplementation(async (command) => ({
      command: { id: "sent-1", sequence: 1, issued_at: "", ...command },
    }));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("explains how to get a desktop when none is taking commands", () => {
    render(<RemoteRoute />);
    publish(desktop({ connected: false, video: undefined }));
    expect(screen.getByText("No desktop is taking commands. On the desktop, take control in MagicHandy and keep that tab in front.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Play" })).not.toBeInTheDocument();
  });

  it("drives the desktop video and shows why a command failed", async () => {
    render(<RemoteRoute />);
    publish(desktop());
    expect(screen.getByText("Connected to the desktop: Videos")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Evening take" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Play" }));
    await waitFor(() => expect(api.sendRemoteCommand).toHaveBeenCalledWith({ target: "video", action: "play" }));
    fireEvent.click(screen.getByRole("button", { name: "Forward 10 s" }));
    await waitFor(() => expect(api.sendRemoteCommand).toHaveBeenLastCalledWith({ target: "video", action: "seek_by", ms: 10_000 }));

    publish(desktop({ revision: 4, recent: [{ command_id: "sent-1", ok: false, error: "The video is not ready for that yet." }] }));
    expect(screen.getByRole("alert")).toHaveTextContent("The video is not ready for that yet.");
  });

  it("opens a catalog video on the desktop", async () => {
    render(<RemoteRoute />);
    publish(desktop({ video: undefined }));
    expect(screen.getByText("The desktop is not showing a video. Open one below.")).toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: /Evening take/ }));
    await waitFor(() => expect(api.sendRemoteCommand).toHaveBeenCalledWith({ target: "video", action: "open", video_id: "take-7" }));
  });

  it("words refused commands itself", async () => {
    vi.mocked(api.sendRemoteCommand).mockRejectedValue(new ApiError("the desktop is not showing that: no video is open on the desktop", 409, { code: "target_unavailable" }));
    render(<RemoteRoute />);
    publish(desktop());
    fireEvent.click(screen.getByRole("button", { name: "Close video" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The desktop is not showing that right now.");
  });

  it("sends chat through the desktop and keeps a message the desktop could not send", async () => {
    vi.mocked(api.getChatMessages).mockResolvedValue({
      session_id: "session-2", latest_seq: 2, cursor: 0,
      messages: [
        { seq: 1, role: "user", content: "Hi there", created_at: "" },
        { seq: 2, role: "assistant", content: "Hello from the desktop", created_at: "" },
      ],
    });
    localStorage.setItem("magichandy-remote-mode", "chat");
    render(<RemoteRoute />);
    publish(desktop({ route: "chat", video: undefined, chat: { session_id: "session-2", persona_name: "Nova", busy: false, ready: true } }));
    expect(await screen.findByText("Hello from the desktop")).toBeInTheDocument();
    expect(screen.getByText("Nova")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Slower please" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(api.sendRemoteCommand).toHaveBeenCalledWith({ target: "chat", action: "send", text: "Slower please" }));
    expect(screen.getByLabelText("Message")).toHaveValue("");

    publish(desktop({ revision: 5, route: "chat", video: undefined, chat: { session_id: "session-2", busy: true, ready: true },
      recent: [{ command_id: "sent-1", ok: false, error: "The desktop is still answering. Send again when the reply finishes." }] }));
    await waitFor(() => expect(screen.getByLabelText("Message")).toHaveValue("Slower please"));
    expect(screen.getByRole("alert")).toHaveTextContent("The desktop is still answering. Send again when the reply finishes.");
  });

  it("offers to open the chat when the desktop shows none", async () => {
    localStorage.setItem("magichandy-remote-mode", "chat");
    render(<RemoteRoute />);
    publish(desktop());
    fireEvent.click(screen.getByRole("button", { name: "Open chat on the desktop" }));
    await waitFor(() => expect(api.sendRemoteCommand).toHaveBeenCalledWith({ target: "chat", action: "open" }));
  });

  it("tells another account and observers what they can do", () => {
    const view = render(<RemoteRoute />);
    publish({ revision: 0, connected: true, other_account: true, pending: 0, recent: [] });
    expect(screen.getByText("The desktop is signed in to another account.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Open a video on the desktop" })).not.toBeInTheDocument();
    view.unmount();

    FakeEventSource.instances = [];
    app.value = { ...app.value, state: { capabilities: { control: false } } };
    render(<RemoteRoute />);
    expect(screen.getByText("This account can watch but not control. Ask the administrator for a control permission to use the remote.")).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(0);
  });
});
