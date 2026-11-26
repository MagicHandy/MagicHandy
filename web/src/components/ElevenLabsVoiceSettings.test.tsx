import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ElevenLabsAccount, ElevenLabsModel, PublicSettings } from "../api/types";
import { elevenLabsStatus, ElevenLabsVoiceSettings } from "./ElevenLabsVoiceSettings";

const mocks = vi.hoisted(() => ({
  api: {
    elevenLabsAccount: vi.fn(),
    elevenLabsModels: vi.fn(),
    elevenLabsVoices: vi.fn(),
    elevenLabsLibrary: vi.fn(),
    elevenLabsAddVoice: vi.fn(),
    elevenLabsPreview: vi.fn(),
  },
  playBlob: vi.fn(),
}));

vi.mock("../api/client", () => ({ api: mocks.api }));
vi.mock("../util/audio", () => ({ playBlob: mocks.playBlob, stopAllAudioPlayback: vi.fn() }));

const jessica = { voice_id: "cgSgspJ2msm6clMCkdW9", name: "Jessica", labels: { gender: "female", age: "young", use_case: "conversational" }, preview_url: "https://storage.googleapis.com/eleven-public-prod/premade/jessica.mp3" };

function supports(overrides: Partial<ElevenLabsModel["supports"]>): ElevenLabsModel["supports"] {
  return { stability: false, similarity: false, style: false, speaker_boost: false, speed: false, language_code: false, audio_tags: false, ...overrides };
}

const models: ElevenLabsModel[] = [
  { model_id: "eleven_multilingual_v2", name: "Eleven Multilingual v2", max_text_length: 10000, supports: supports({ stability: true, similarity: true, style: true, speaker_boost: true, speed: true }) },
  { model_id: "eleven_v4_turbo", name: "Eleven v4 Turbo", max_text_length: 100000, languages: [{ language_id: "en", name: "English" }, { language_id: "ja", name: "Japanese" }], supports: supports({ stability: true, similarity: true, language_code: true, audio_tags: true }) },
];

function account(overrides: Partial<ElevenLabsAccount> = {}): ElevenLabsAccount {
  return {
    key_set: true,
    key_status: "ok",
    permissions: { user_read: { state: "ok" }, models_read: { state: "ok" }, voices_read: { state: "ok" } },
    subscription: { tier: "free", status: "active", character_count: 1200, character_limit: 10000, next_reset_unix: null, voice_slots: 0, voice_limit: 3 },
    library_via_api: false,
    selected_voice: jessica,
    ...overrides,
  };
}

function voiceSettings(overrides: Partial<PublicSettings["voice"]> = {}): PublicSettings["voice"] {
  return { elevenlabs_key_set: true, elevenlabs_voice_id: jessica.voice_id, elevenlabs_model_id: "eleven_v4_turbo", ...overrides } as PublicSettings["voice"];
}

function renderSettings(voice = voiceSettings(), props: { patch?: ReturnType<typeof vi.fn>; newKey?: string } = {}) {
  const patch = props.patch ?? vi.fn();
  render(<ElevenLabsVoiceSettings voice={voice} locked={false} patch={patch} newKey={props.newKey ?? ""} setNewKey={vi.fn()} clearKey={false} setClearKey={vi.fn()} />);
  return patch;
}

beforeEach(() => {
  for (const fn of Object.values(mocks.api)) fn.mockReset();
  mocks.playBlob.mockReset();
  mocks.api.elevenLabsModels.mockResolvedValue({ models });
});

describe("elevenLabsStatus", () => {
  it("explains each key state in plain words", () => {
    expect(elevenLabsStatus(null, false, "", false).label).toBe("No API key");
    expect(elevenLabsStatus(null, false, "", true).label).toBe("Not checked yet");
    expect(elevenLabsStatus(null, true, "", true).label).toBe("Checking…");
    const connected = elevenLabsStatus(account(), false, "", true);
    expect(connected.label).toBe("Connected");
    expect(connected.detail).toBe("Free plan · 1,200 of 10,000 credits used");
    expect(elevenLabsStatus(account({ subscription: { ...account().subscription!, tier: "creator", character_count: 10000 } }), false, "", true).label).toBe("Out of credits");
    expect(elevenLabsStatus(account({ key_status: "restricted" }), false, "", true).label).toBe("Speech only");
    expect(elevenLabsStatus(account({ key_status: "invalid" }), false, "", true).detail).toBe("ElevenLabs didn't accept this key. Check or replace it.");
    expect(elevenLabsStatus(account({ key_status: "unusual_activity", message: "Free use is paused." }), false, "", true)).toMatchObject({ label: "Free use paused", detail: "Free use is paused.", dot: "error" });
  });
});

describe("ElevenLabsVoiceSettings", () => {
  it("checks the saved key, shows the plan and offers only the controls the model honors", async () => {
    mocks.api.elevenLabsAccount.mockResolvedValue(account());
    renderSettings();
    expect(await screen.findByText("Free plan · 1,200 of 10,000 credits used")).toBeInTheDocument();
    expect(mocks.api.elevenLabsAccount).toHaveBeenCalledWith(undefined);
    expect(screen.getByText("Jessica")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Change voice…" })).toBeEnabled();

    const model = screen.getByRole("combobox", { name: "Model" });
    await waitFor(() => expect(within(model).getAllByRole("option").map((option) => option.getAttribute("value"))).toEqual(["eleven_v4_turbo", "eleven_multilingual_v2"]));
    expect(model).toHaveValue("eleven_v4_turbo");

    expect(screen.getByRole("slider", { name: "Stability" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Similarity" })).toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Style" })).not.toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Speed" })).not.toBeInTheDocument();
    expect(screen.getByText("Controls this model ignores are hidden, and settings it ignores are never sent.")).toBeInTheDocument();
    expect(within(screen.getByRole("combobox", { name: "Language" })).getByRole("option", { name: "Japanese" })).toBeInTheDocument();
  });

  it("falls back to a voice ID field for a key that can only speak", async () => {
    mocks.api.elevenLabsAccount.mockResolvedValue(account({ key_status: "restricted", permissions: { voices_read: { state: "denied" } }, subscription: undefined }));
    renderSettings();
    expect(await screen.findByText("Speech only")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Change voice…" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Voice" })).toHaveValue(jessica.voice_id);
    expect(screen.getByText("Copy the voice ID from the voice's page in ElevenLabs.")).toBeInTheDocument();
    expect(mocks.api.elevenLabsModels).not.toHaveBeenCalled();
  });

  it("checks a typed key without saving it", async () => {
    mocks.api.elevenLabsAccount.mockResolvedValue(account({ key_status: "invalid", message: "" }));
    renderSettings(voiceSettings({ elevenlabs_key_set: false }), { newKey: "  sk_typed_key  " });
    expect(screen.getByText("No API key")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Check key" }));
    expect(await screen.findByText("Key not accepted")).toBeInTheDocument();
    expect(mocks.api.elevenLabsAccount).toHaveBeenCalledWith({ api_key: "sk_typed_key", voice_id: jessica.voice_id });
    expect(mocks.api.elevenLabsModels).not.toHaveBeenCalled();
  });

  it("saves delivery overrides and resets them to the voice's own setting", async () => {
    mocks.api.elevenLabsAccount.mockResolvedValue(account());
    const patch = renderSettings(voiceSettings({ elevenlabs_model_id: "eleven_multilingual_v2", elevenlabs_delivery: { speed: 1.1 } }));
    await screen.findByText("Free plan · 1,200 of 10,000 credits used");
    expect(screen.getAllByText("Voice setting").length).toBeGreaterThan(0);
    fireEvent.change(screen.getByRole("slider", { name: "Stability" }), { target: { value: "0.3" } });
    expect(patch).toHaveBeenLastCalledWith({ elevenlabs_delivery: { speed: 1.1, stability: 0.3 } });
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(patch).toHaveBeenLastCalledWith({ elevenlabs_delivery: {} });
  });

  it("reports a preview that cannot play", async () => {
    mocks.api.elevenLabsAccount.mockResolvedValue(account());
    mocks.api.elevenLabsPreview.mockRejectedValue(new Error("The preview could not play."));
    renderSettings();
    await screen.findByText("Jessica");
    fireEvent.click(screen.getByRole("button", { name: "Play a preview of Jessica" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The preview could not play.");
    expect(mocks.api.elevenLabsPreview).toHaveBeenCalledWith(jessica.preview_url);
    expect(mocks.playBlob).not.toHaveBeenCalled();
  });
});
