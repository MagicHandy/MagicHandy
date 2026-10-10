import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { NoticePreferencesProvider } from "../state/notice-preferences";
import { newModelConnection } from "../util/model-connections";
import { HostedModelConnection } from "./HostedModelConnection";

afterEach(() => vi.restoreAllMocks());

it.each(["Just this time", "Don't show again"])("dismisses provider help with %s while credentials remain locked", async lifetime => {
  vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue({ connection: { generation: 0, profiles: [] } } as unknown as CloudPlanningStatus);
  vi.spyOn(api, "noticePreferences").mockResolvedValue({ scope: "browser", hidden: [] });
  const saveNotice = vi.spyOn(api, "saveNoticePreference").mockResolvedValue({ scope: "browser", hidden: ["model-api-credentials"] });
  const saveKey = vi.spyOn(api, "modelConnectionKey");
  const patch = vi.fn();
  render(<NoticePreferencesProvider enabled scope="browser"><HostedModelConnection connection={newModelConnection("openrouter", [])} locked patch={patch} /></NoticePreferencesProvider>);
  const dismiss = await screen.findByRole("button", { name: "Dismiss API key guidance" });
  expect(dismiss).toBeEnabled();
  expect(screen.getByLabelText("API key")).toBeDisabled();
  fireEvent.click(dismiss);
  fireEvent.click(screen.getByRole("button", { name: lifetime }));
  await waitFor(() => expect(screen.queryByRole("note", { name: "API key guidance" })).not.toBeInTheDocument());
  expect(screen.getByLabelText("API key")).toBeVisible();
  expect(screen.getByLabelText("API key")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save API key" })).toBeDisabled();
  expect(saveKey).not.toHaveBeenCalled();
  expect(patch).not.toHaveBeenCalled();
  expect(saveNotice).toHaveBeenCalledTimes(lifetime === "Don't show again" ? 1 : 0);
});
