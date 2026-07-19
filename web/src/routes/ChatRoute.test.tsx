import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ChatRoute } from "./ChatRoute";

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
vi.mock("../components/WorkspaceHead", () => ({
  WorkspaceHead: () => <h1>Chat</h1>,
}));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ motion: {} }),
}));

describe("ChatRoute", () => {
  it("keeps Autopilot in the control sidebar and manual testing out of Chat", () => {
    render(<ChatRoute />);

    const conversation = screen.getByRole("region", { name: "Conversation" });
    const controls = screen.getByRole("complementary", { name: "Motion controls" });
    expect(within(conversation).getByText("Conversation content")).toBeInTheDocument();
    expect(within(conversation).queryByText("Autopilot control")).not.toBeInTheDocument();
    expect(within(controls).getByText("Autopilot control")).toBeInTheDocument();
    expect(screen.queryByText("Manual motion")).not.toBeInTheDocument();
  });
});
