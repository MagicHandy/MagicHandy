import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { CloudCredentialRecovery } from "./CloudCredentialRecovery";
import { ChatGPTConnection } from "./ChatGPTConnection";
import { ModelConnectionEditor } from "./ModelConnectionEditor";
import { newModelConnection } from "../util/model-connections";

const status: CloudPlanningStatus = {
  connection: { profiles: [], active: "", pending: false, state: "disconnected", generation: 2, decisions_key_set: false },
  models: [], readiness: { provider: "", model: "", ready: false, state: "untested" }, motion_planner: { provider: "conversation", model: "" },
};

describe("cloud credential recovery", () => {
  afterEach(() => vi.restoreAllMocks());

  it("requires confirmation of all affected accounts and supports cancel", async () => {
    const reset = vi.spyOn(api, "cloudResetCredentials").mockResolvedValue(status);
    const onReset = vi.fn();
    render(<CloudCredentialRecovery locked={false} onReset={onReset} />);
    fireEvent.click(screen.getByRole("button", { name: "Reset saved credentials…" }));
    expect(screen.getByRole("status")).toHaveTextContent("all saved ChatGPT sign-ins and cloud API keys");
    expect(reset).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(reset).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Reset saved credentials…" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset all saved cloud credentials" }));
    await waitFor(() => expect(onReset).toHaveBeenCalledWith(status));
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("shows a failed reset and keeps read-only clients from mutating credentials", async () => {
    vi.spyOn(api, "cloudResetCredentials").mockRejectedValue(new Error("refresh settings"));
    const onReset = vi.fn();
    const { rerender } = render(<CloudCredentialRecovery locked={false} onReset={onReset} />);
    fireEvent.click(screen.getByRole("button", { name: "Reset saved credentials…" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset all saved cloud credentials" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("refresh settings");
    expect(onReset).not.toHaveBeenCalled();
    rerender(<CloudCredentialRecovery locked onReset={onReset} />);
    expect(screen.getByRole("button", { name: "Reset all saved cloud credentials" })).toBeDisabled();
  });

  it("replaces unusable setup sign-in with recovery and returns to sign-in after reset", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue({ ...status, connection: { ...status.connection, state: "unavailable", recovery_required: true } });
    vi.spyOn(api, "cloudResetCredentials").mockResolvedValue(status);
    render(<ChatGPTConnection presentation="setup" />);
    fireEvent.click(await screen.findByRole("button", { name: "Reset saved credentials…" }));
    expect(screen.queryByRole("button", { name: "Continue with ChatGPT" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Reset all saved cloud credentials" }));
    expect(await screen.findByRole("button", { name: "Continue with ChatGPT" })).toBeEnabled();
  });

  it("recovers an API provider editor without discovering models or claiming readiness", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue({ ...status, connection: { ...status.connection, state: "unavailable", recovery_required: true } });
    vi.spyOn(api, "cloudResetCredentials").mockResolvedValue(status);
    const models = vi.spyOn(api, "modelConnectionModels");
    const ready = vi.fn();
    render(<ModelConnectionEditor connection={newModelConnection("openrouter", [])} locked={false} patch={vi.fn()} onReady={ready} />);
    fireEvent.click(await screen.findByRole("button", { name: "Reset saved credentials…" }));
    expect(models).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Reset all saved cloud credentials" }));
    expect(await screen.findByLabelText("API key")).toHaveValue("");
    expect(screen.getByRole("button", { name: "Connect provider" })).toBeDisabled();
    expect(ready).not.toHaveBeenCalledWith(true);
  });
});
