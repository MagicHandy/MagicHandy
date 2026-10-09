import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { ChatGPTConnection } from "./ChatGPTConnection";

const disconnected: CloudPlanningStatus = {
  connection: { active: "", profiles: [], pending: false, state: "disconnected", generation: 0, decisions_key_set: false },
  models: [], readiness: { provider: "", model: "", ready: false, state: "untested" }, motion_planner: { provider: "conversation", model: "" },
};
const connected: CloudPlanningStatus = { ...disconnected, connection: {
  ...disconnected.connection, active: "test", state: "connected", generation: 1,
  profiles: [{ id: "test", label: "Test account", connected: true, plan_authorized: true, welcome_pending: false }],
} };

describe("ChatGPT account presentation", () => {
  beforeEach(() => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(disconnected);
    vi.spyOn(api, "cloudSignIn").mockResolvedValue({ authorization_url: "https://auth.openai.com/authorize" });
    vi.spyOn(api, "modelConnectionTest");
  });
  afterEach(() => vi.restoreAllMocks());

  it("shows one sign-in action and does not create a backend attempt if popups are blocked", async () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    render(<ChatGPTConnection presentation="setup" />);
    fireEvent.click(await screen.findByRole("button", { name: "Continue with ChatGPT" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Allow a sign-in window");
    expect(api.cloudSignIn).not.toHaveBeenCalled();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
  });

  it("shows pending sign-in with cancellation, without a second sign-in button", async () => {
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...disconnected, connection: { ...disconnected.connection, pending: true } });
    render(<ChatGPTConnection presentation="setup" />);
    await screen.findByText("Complete sign-in in the opened window.");
    expect(screen.getByRole("button", { name: "Cancel sign-in" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Continue with ChatGPT" })).not.toBeInTheDocument();
  });

  it("collapses connected account tools and uses a note for the plan acknowledgement", async () => {
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...connected, connection: { ...connected.connection, profiles: [{ ...connected.connection.profiles[0], welcome_pending: true }] } });
    render(<ChatGPTConnection presentation="setup" />);
    await screen.findByText("ChatGPT connected");
    expect(screen.getByText("Manage account").parentElement).not.toHaveAttribute("open");
    expect(screen.getByRole("note")).toHaveTextContent("You're using your ChatGPT plan");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
  });

  it.each(["expired", "permission"])("offers Reconnect for %s without claiming readiness", async state => {
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...connected, connection: {
      ...connected.connection, state, profiles: [{ ...connected.connection.profiles[0], connected: state !== "expired", plan_authorized: state !== "permission" }],
    } });
    render(<ChatGPTConnection presentation="setup" />);
    await screen.findByRole("button", { name: "Reconnect" });
    expect(screen.queryByText("ChatGPT connected")).not.toBeInTheDocument();
  });

  it("keeps a read-only account visible and disables account mutations", async () => {
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue(connected);
    render(<ChatGPTConnection presentation="setup" locked />);
    await screen.findByText("ChatGPT connected");
    fireEvent.click(screen.getByText("Manage account"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Disconnect" })).toBeDisabled());
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add account" })).toBeDisabled();
  });
});
