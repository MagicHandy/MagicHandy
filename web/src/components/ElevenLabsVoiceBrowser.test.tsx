import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ElevenLabsLibraryVoice } from "../api/types";
import { ElevenLabsVoiceBrowser } from "./ElevenLabsVoiceBrowser";

const mocks = vi.hoisted(() => ({
  api: {
    elevenLabsVoices: vi.fn(),
    elevenLabsLibrary: vi.fn(),
    elevenLabsAddVoice: vi.fn(),
  },
}));

vi.mock("../api/client", () => ({ api: mocks.api }));

const mine = [
  { voice_id: "cgSgspJ2msm6clMCkdW9", name: "Jessica", category: "premade", labels: { gender: "female", age: "young" }, preview_url: "https://storage.googleapis.com/eleven-public-prod/jessica.mp3" },
  { voice_id: "pFZP5JQG7iQjIQuC4Bku", name: "Lily", category: "premade", labels: { gender: "female", accent: "british" }, preview_url: "https://storage.googleapis.com/eleven-public-prod/lily.mp3", is_legacy: true },
];

const libraryVoice: ElevenLabsLibraryVoice = {
  public_owner_id: "owner-1",
  voice_id: "lib-voice-1",
  name: "Velvet",
  gender: "female",
  age: "middle_aged",
  descriptive: "soft",
  category: "high_quality",
  preview_url: "https://storage.googleapis.com/eleven-public-prod/velvet.mp3",
  free_users_allowed: true,
  live_moderation_enabled: false,
  rate: 2,
};

function renderBrowser(props: Partial<Parameters<typeof ElevenLabsVoiceBrowser>[0]> = {}) {
  const handlers = { onClose: vi.fn(), onChoose: vi.fn(), onPreview: vi.fn() };
  render(<ElevenLabsVoiceBrowser currentVoiceID="cgSgspJ2msm6clMCkdW9" canAddVoices libraryViaAPI {...handlers} {...props} />);
  return handlers;
}

beforeEach(() => {
  for (const fn of Object.values(mocks.api)) fn.mockReset();
  mocks.api.elevenLabsVoices.mockResolvedValue({ voices: mine, has_more: false });
  mocks.api.elevenLabsLibrary.mockResolvedValue({ voices: [libraryVoice], has_more: false });
});

describe("ElevenLabsVoiceBrowser", () => {
  it("lists the account's voices and confirms a new choice", async () => {
    const { onChoose, onPreview } = renderBrowser();
    const lily = (await screen.findByText("Lily")).closest("li")!;
    expect(within(lily).getByText("Legacy")).toBeInTheDocument();
    expect(within(screen.getByText("Jessica").closest("li")!).getByText("Selected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Use voice" })).toBeDisabled();

    fireEvent.click(within(lily).getByRole("button", { name: "Play a preview of Lily" }));
    expect(onPreview).toHaveBeenCalledWith(mine[1].preview_url);
    fireEvent.click(within(lily).getByRole("button", { name: "Use" }));
    fireEvent.click(screen.getByRole("button", { name: "Use voice" }));
    expect(onChoose).toHaveBeenCalledWith(expect.objectContaining({ voice_id: "pFZP5JQG7iQjIQuC4Bku", name: "Lily" }));
    expect(mocks.api.elevenLabsVoices).toHaveBeenCalledWith({ page_size: "30", sort: "name" }, expect.any(AbortSignal));
  });

  it("searches the library with companion presets and hides screened and surcharged voices by default", async () => {
    renderBrowser();
    fireEvent.click(screen.getByRole("tab", { name: "Voice Library" }));
    const row = (await screen.findByText("Velvet")).closest("li")!;
    expect(mocks.api.elevenLabsLibrary).toHaveBeenLastCalledWith({ search: "intimate", sort: "trending", page_size: "30", language: "en" }, expect.any(AbortSignal));
    expect(within(row).getByText("Studio quality")).toBeInTheDocument();
    expect(within(row).getByText("2× credits")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Conversational" }));
    await waitFor(() => expect(mocks.api.elevenLabsLibrary).toHaveBeenLastCalledWith({ use_cases: "conversational", sort: "trending", page_size: "30", language: "en" }, expect.any(AbortSignal)));

    fireEvent.click(screen.getByRole("checkbox", { name: "Hide voices that screen what they say" }));
    await waitFor(() => expect(mocks.api.elevenLabsLibrary).toHaveBeenLastCalledWith(expect.objectContaining({ include_live_moderated: "true" }), expect.any(AbortSignal)));
  });

  it("adds a library voice to the account and uses it", async () => {
    mocks.api.elevenLabsAddVoice.mockResolvedValue({ voice_id: "added-voice-id" });
    const { onChoose } = renderBrowser();
    fireEvent.click(screen.getByRole("tab", { name: "Voice Library" }));
    const row = (await screen.findByText("Velvet")).closest("li")!;
    fireEvent.click(within(row).getByRole("button", { name: "Add & use" }));
    await waitFor(() => expect(onChoose).toHaveBeenCalledWith(expect.objectContaining({ voice_id: "added-voice-id", name: "Velvet", preview_url: libraryVoice.preview_url })));
    expect(mocks.api.elevenLabsAddVoice).toHaveBeenCalledWith({ public_owner_id: "owner-1", voice_id: "lib-voice-1", name: "Velvet" });
  });

  it("explains that a free plan cannot use library voices instead of offering to add them", async () => {
    renderBrowser({ libraryViaAPI: false });
    fireEvent.click(screen.getByRole("tab", { name: "Voice Library" }));
    const row = (await screen.findByText("Velvet")).closest("li")!;
    expect(screen.getByText("Library voices need a paid plan here.")).toBeInTheDocument();
    expect(within(row).getByText("Needs a paid plan")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "Add & use" })).not.toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Play a preview of Velvet" })).toBeEnabled();
  });

  it("closes on Escape", async () => {
    const { onClose } = renderBrowser();
    await screen.findByText("Jessica");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });
});
