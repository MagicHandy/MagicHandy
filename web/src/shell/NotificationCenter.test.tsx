import { render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import { NotificationCenter } from "./NotificationCenter";

const app = vi.hoisted(() => ({
  mode: "automatic",
  push: vi.fn(),
  voiceEnabled: false,
  workers: undefined as Record<string, { state: string; started_at?: string }> | undefined,
}));

vi.mock("../api/client", () => ({
  api: { updateStatus: vi.fn() },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    backendOnline: true,
    startupError: "",
    state: {
      version: "1.0.0",
      settings: {
        ui: { setup_completed: true, update_check_mode: app.mode },
        voice: { enabled: app.voiceEnabled },
      },
      voice: { workers: app.workers },
    },
  }),
  useNotifications: () => ({
    items: [],
    unreadCount: 0,
    push: app.push,
    markRead: vi.fn(),
    markAllRead: vi.fn(),
    clear: vi.fn(),
  }),
}));

const updateStatus = vi.mocked(api.updateStatus);

describe("NotificationCenter release checks", () => {
  beforeEach(() => {
    setLocaleForTest("en", english);
    app.mode = "automatic";
    app.voiceEnabled = false;
    app.workers = undefined;
    app.push.mockReset();
    updateStatus.mockReset();
    updateStatus.mockResolvedValue({ state: "current", current_version: "1.0.0" });
  });

  it("records one actionable notification for an available stable release", async () => {
    updateStatus.mockResolvedValue({
      state: "available",
      current_version: "1.0.0",
      latest: { version: "1.1.0", tag: "v1.1.0", url: "https://github.com/MagicHandy/MagicHandy/releases/tag/v1.1.0" },
    });
    render(<NotificationCenter open={false} onOpenChange={vi.fn()} />);

    await waitFor(() => expect(app.push).toHaveBeenCalledWith(expect.objectContaining({
      title: "New MagicHandy release",
      href: "#/settings/general",
      sourceKey: "update-available:v1.1.0",
    })));
  });

  it("does not contact GitHub when checks are manual only", () => {
    app.mode = "manual";
    render(<NotificationCenter open={false} onOpenChange={vi.fn()} />);

    expect(updateStatus).not.toHaveBeenCalled();
  });

  it("records a voice worker crash without treating startup loading as history", async () => {
    app.voiceEnabled = true;
    app.workers = {
      tts: { state: "crashed", started_at: "2026-08-02T12:00:00Z" },
      asr: { state: "starting", started_at: "2026-08-02T12:00:01Z" },
    };

    render(<NotificationCenter open={false} onOpenChange={vi.fn()} />);

    await waitFor(() => expect(app.push).toHaveBeenCalledWith(expect.objectContaining({
      title: "Voice worker crashed",
      category: "voice",
      href: "#/settings/voice",
      sourceKey: "voice-worker-crashed:tts:2026-08-02T12:00:00Z",
    })));
    expect(app.push).not.toHaveBeenCalledWith(expect.objectContaining({
      sourceKey: expect.stringContaining("asr"),
    }));
  });
});
