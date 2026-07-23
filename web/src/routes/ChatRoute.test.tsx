import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ChatRoute } from "./ChatRoute";

const mocks = vi.hoisted(() => ({
  getChatSessions: vi.fn(),
  createChatSession: vi.fn(),
  activateChatSession: vi.fn(),
  saveChatSession: vi.fn(),
  deleteChatSession: vi.fn(),
  stopMode: vi.fn(),
  show: vi.fn(),
  refresh: vi.fn(),
  appState: { modes: {} } as {
    modes: { mode?: string };
    chat?: { active_session_id?: string; latest_seq?: number; current_mood?: string };
    uptime_seconds?: number;
  },
}));

const current = (messageCount = 0, saved = false) => ({
  active_session_id: "chat-test",
  sessions: [{ id: "chat-test", title: "Current chat", saved, active: true, message_count: messageCount, latest_seq: messageCount, created_at: "now", updated_at: "now" }],
});

async function openNewChatDialog() {
  const button = await screen.findByRole("button", { name: "Start a new chat" });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  return screen.findByRole("dialog", { name: "Start a new chat?" });
}

vi.mock("../components/AutopilotControl", () => ({
  AutopilotControl: () => <div>Autopilot control</div>,
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: () => <div>Conversation content</div>,
}));
vi.mock("../components/MotionVisualizer", () => ({
  MotionVisualizer: () => <div>Motion visualizer</div>,
}));
vi.mock("../components/QuickSettings", () => ({
  QuickSettings: () => <div>Quick settings</div>,
}));
vi.mock("../components/VoiceQuickControls", () => ({
  VoiceQuickControls: () => <div>Voice controls</div>,
}));
vi.mock("../api/client", () => ({
  api: {
    getChatSessions: mocks.getChatSessions,
    createChatSession: mocks.createChatSession,
    activateChatSession: mocks.activateChatSession,
    saveChatSession: mocks.saveChatSession,
    deleteChatSession: mocks.deleteChatSession,
    stopMode: mocks.stopMode,
  },
}));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ backendOnline: true, readOnly: false, state: mocks.appState, motion: {}, refresh: mocks.refresh }),
  useToast: () => ({ show: mocks.show }),
}));

describe("ChatRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.appState = { modes: {} };
    mocks.getChatSessions.mockResolvedValue(current());
    mocks.saveChatSession.mockResolvedValue(current(2, true));
    mocks.createChatSession.mockResolvedValue({
      active_session_id: "chat-new",
      sessions: [{ id: "chat-new", title: "New chat", saved: false, active: true, message_count: 0, latest_seq: 0, created_at: "now", updated_at: "now" }],
    });
  });

  it("keeps Autopilot in the control sidebar and manual testing out of Chat", async () => {
    render(<ChatRoute />);

    const conversation = screen.getByRole("region", { name: "Conversation" });
    const controls = screen.getByRole("complementary", { name: "Motion controls" });
    const title = within(conversation).getByRole("heading", { name: "Chat", level: 1 });
    expect(title.closest(".chat-tabs-bar")).toContainElement(within(conversation).getByRole("tablist", { name: "Chat sessions" }));
    expect(await within(conversation).findByText("Conversation content")).toBeInTheDocument();
    expect(within(conversation).queryByText("Autopilot control")).not.toBeInTheDocument();
    expect(within(controls).getByText("Autopilot control")).toBeInTheDocument();
    expect(screen.queryByText("Manual motion")).not.toBeInTheDocument();
  });

  it("shows only the backend mood for the active session", async () => {
    mocks.appState = { modes: {}, chat: { active_session_id: "chat-test", current_mood: "Teasing" } };
    const view = render(<ChatRoute />);

    expect(await screen.findByRole("status", { name: "Assistant mood: Teasing" })).toHaveTextContent("MoodTeasing");
    mocks.appState = { modes: {}, chat: { active_session_id: "chat-test", current_mood: "Curious" } };
    view.rerender(<ChatRoute />);
    expect(screen.getByRole("status", { name: "Assistant mood: Curious" })).toBeInTheDocument();

    mocks.appState = { modes: {}, chat: { active_session_id: "another-chat", current_mood: "Playful" } };
    view.rerender(<ChatRoute />);
    expect(screen.queryByRole("status", { name: /Assistant mood/ })).not.toBeInTheDocument();
  });

  it("confirms a new chat and can save the active conversation first", async () => {
    mocks.getChatSessions.mockResolvedValueOnce(current(2, false));
    render(<ChatRoute />);

    expect(await openNewChatDialog()).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save and start" }));

    await waitFor(() => {
      expect(mocks.saveChatSession).toHaveBeenCalledWith("chat-test");
      expect(mocks.createChatSession).toHaveBeenCalledWith(false);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  it("confirms replacing an empty active chat", async () => {
    mocks.getChatSessions.mockResolvedValueOnce(current(0, false));
    render(<ChatRoute />);

    expect(await openNewChatDialog()).toHaveTextContent("current chat is empty");
    expect(mocks.createChatSession).not.toHaveBeenCalled();
  });

  it("discards an unsaved working tab when switching without saving", async () => {
    const workspace = current(2, false);
    workspace.sessions.push({
      id: "chat-saved",
      title: "Saved chat",
      saved: true,
      active: false,
      message_count: 4,
      latest_seq: 9,
      created_at: "earlier",
      updated_at: "earlier",
    });
    mocks.getChatSessions.mockResolvedValueOnce(workspace);
    mocks.activateChatSession.mockResolvedValue({
      active_session_id: "chat-saved",
      sessions: [{ ...workspace.sessions[1], active: true }],
    });
    render(<ChatRoute />);

    fireEvent.click(await screen.findByRole("tab", { name: "Saved chat" }));
    fireEvent.click(screen.getByRole("button", { name: "Switch without saving" }));

    await waitFor(() => expect(mocks.activateChatSession).toHaveBeenCalledWith("chat-saved", true));
  });

  it("refreshes the tab workspace when another client changes the backend-active chat", async () => {
    const initial = current(1, true);
    initial.sessions.push({
      id: "chat-saved",
      title: "Other saved chat",
      saved: true,
      active: false,
      message_count: 3,
      latest_seq: 8,
      created_at: "earlier",
      updated_at: "earlier",
    });
    const switched = {
      active_session_id: "chat-saved",
      sessions: initial.sessions.map((session) => ({ ...session, active: session.id === "chat-saved" })),
    };
    mocks.appState = { modes: {}, chat: { active_session_id: "chat-test", latest_seq: 1 }, uptime_seconds: 1 };
    mocks.getChatSessions.mockResolvedValueOnce(initial).mockResolvedValue(switched);
    const view = render(<ChatRoute />);

    expect(await screen.findByRole("tab", { name: "Current chat" })).toHaveAttribute("aria-selected", "true");
    mocks.appState = { modes: {}, chat: { active_session_id: "chat-saved", latest_seq: 8 }, uptime_seconds: 2 };
    view.rerender(<ChatRoute />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "Other saved chat" })).toHaveAttribute("aria-selected", "true"));
    expect(mocks.getChatSessions).toHaveBeenCalledTimes(2);
  });
});
