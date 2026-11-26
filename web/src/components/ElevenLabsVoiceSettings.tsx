import { useCallback, useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { ElevenLabsAccount, ElevenLabsDelivery, ElevenLabsModel, PublicSettings } from "../api/types";
import { formatNumber, t, translateKnown, type MessageKey } from "../i18n";
import { playBlob, stopAllAudioPlayback } from "../util/audio";
import { RangeInput } from "./RangeInput";
import { ElevenLabsVoiceBrowser, type ChosenVoice } from "./ElevenLabsVoiceBrowser";
// Loaded with the Settings chunk; nothing outside Settings > Voice uses it.
import "../styles/elevenlabs.css";

// The ElevenLabs part of Settings > Voice > Speech output. The backend reads
// the account, models and voices with the saved key (or one typed here for the
// check) and never returns the key; this view only renders those answers.

const KEY_PAGE = "https://elevenlabs.io/app/settings/api-keys";

const MODEL_ORDER = ["eleven_v4_turbo", "eleven_v4", "eleven_v3", "eleven_multilingual_v2", "eleven_flash_v2_5"];
const MODEL_LABELS: Partial<Record<string, MessageKey>> = {
  eleven_v4_turbo: "Eleven v4 Turbo — expressive and fast",
  eleven_v4: "Eleven v4 — most expressive",
  eleven_v3: "Eleven v3 — expressive",
  eleven_multilingual_v2: "Multilingual v2 — steady",
  eleven_flash_v2_5: "Flash v2.5 — fastest",
};
const MODEL_HINTS: Partial<Record<string, MessageKey>> = {
  eleven_v4_turbo: "Natural, expressive delivery that starts speaking quickly, at half the credits of Eleven v4.",
  eleven_v4: "The most natural delivery. Replies start a little later and use full credits.",
  eleven_v3: "Expressive delivery from the previous generation.",
  eleven_multilingual_v2: "Steady delivery, with style and speed controls.",
  eleven_flash_v2_5: "Starts speaking fastest at half the credits, with less expressive delivery.",
};

type Supports = ElevenLabsModel["supports"];
type DeliveryKey = "stability" | "similarity" | "style" | "speed";

// Used only until the account's own model list arrives (or when the key
// cannot read it); it mirrors what the worker sends for each model.
function fallbackSupports(id: string): Supports {
  const base = { stability: true, similarity: true, style: false, speaker_boost: false, speed: false, language_code: false, audio_tags: false };
  if (id.startsWith("eleven_v4") || id.startsWith("eleven_v3")) return { ...base, language_code: true, audio_tags: true };
  if (id === "eleven_multilingual_v2") return { ...base, style: true, speaker_boost: true, speed: true };
  if (id.startsWith("eleven_flash_v2") || id.startsWith("eleven_turbo_v2")) return { ...base, speed: true, language_code: id.endsWith("_5") };
  return base;
}

const DELIVERY: { key: DeliveryKey; label: MessageKey; hint: MessageKey; min: number; max: number; step: number; neutral: number }[] = [
  { key: "stability", label: "Stability", hint: "Lower varies the delivery more; higher keeps it steady.", min: 0, max: 1, step: 0.05, neutral: 0.5 },
  { key: "similarity", label: "Similarity", hint: "How closely speech follows the original voice.", min: 0, max: 1, step: 0.05, neutral: 0.75 },
  { key: "style", label: "Style", hint: "Exaggerates the voice's own style. Higher values can add delay.", min: 0, max: 1, step: 0.05, neutral: 0 },
  { key: "speed", label: "Speed", hint: "1.0 is the voice's natural pace.", min: 0.7, max: 1.2, step: 0.05, neutral: 1 },
];

interface StatusView {
  dot: "idle" | "active" | "ok" | "warn" | "error";
  label: string;
  detail: string;
}

function planName(tier: string): string {
  if (tier.toLowerCase() === "free") return t("Free plan");
  const name = tier.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
  return t("{plan} plan", { plan: name });
}

function resetDate(unix?: number | null): string {
  if (!unix) return "";
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(new Date(unix * 1000));
}

export function elevenLabsStatus(account: ElevenLabsAccount | null, checking: boolean, failure: string, keySet: boolean): StatusView {
  if (checking) return { dot: "active", label: t("Checking…"), detail: t("Reading your plan, models and voices. This doesn't use credits.") };
  if (failure) return { dot: "error", label: t("ElevenLabs check failed"), detail: failure };
  if (!account || !account.key_set || account.key_status === "missing" || !account.key_status) {
    return keySet && !account?.key_status
      ? { dot: "idle", label: t("Not checked yet"), detail: t("Check the key to see your plan and voices.") }
      : { dot: "idle", label: t("No API key"), detail: t("Paste an ElevenLabs API key to choose voices and speak replies.") };
  }
  const message = account.message ? translateKnown(account.message) : "";
  switch (account.key_status) {
    case "ok": {
      const plan = account.subscription;
      if (!plan) return { dot: "ok", label: t("Connected"), detail: t("This key can't read the plan and credits.") };
      const reset = resetDate(plan.next_reset_unix);
      if (plan.character_limit > 0 && plan.character_count >= plan.character_limit) {
        return { dot: "warn", label: t("Out of credits"), detail: reset ? t("Speech resumes when credits reset on {date}, or add credits in ElevenLabs.", { date: reset }) : t("Add credits in ElevenLabs to keep speaking.") };
      }
      const usage = t("{used} of {limit} credits used", { used: formatNumber(plan.character_count), limit: formatNumber(plan.character_limit) });
      return { dot: "ok", label: t("Connected"), detail: [planName(plan.tier), usage, reset ? t("resets {date}", { date: reset }) : ""].filter(Boolean).join(" · ") };
    }
    case "restricted":
      return { dot: "warn", label: t("Speech only"), detail: t("This key can speak but can't list voices or show usage. Give it Voices (read) and User (read) in ElevenLabs to browse here.") };
    case "invalid":
      return { dot: "error", label: t("Key not accepted"), detail: message || t("ElevenLabs didn't accept this key. Check or replace it.") };
    case "unusual_activity":
      return { dot: "error", label: t("Free use paused"), detail: message };
    case "unreachable":
      return { dot: "error", label: t("Can't reach ElevenLabs"), detail: message || t("Check the internet connection.") };
    default:
      return { dot: "error", label: t("ElevenLabs check failed"), detail: message };
  }
}

function voiceLabels(labels?: Record<string, string> | null): string[] {
  if (!labels) return [];
  return ["gender", "age", "accent", "descriptive", "use_case"]
    .map((key) => labels[key])
    .filter((value): value is string => Boolean(value && value.trim()))
    .map((value) => value.replace(/_/g, " "));
}

interface Props {
  voice: PublicSettings["voice"];
  locked: boolean;
  patch: (next: Partial<PublicSettings["voice"]>) => void;
  newKey: string;
  setNewKey: (value: string) => void;
  clearKey: boolean;
  setClearKey: (value: boolean) => void;
}

export function ElevenLabsVoiceSettings({ voice, locked, patch, newKey, setNewKey, clearKey, setClearKey }: Props) {
  const baseID = useId();
  const [account, setAccount] = useState<ElevenLabsAccount | null>(null);
  const [checking, setChecking] = useState(false);
  const [failure, setFailure] = useState("");
  const [models, setModels] = useState<ElevenLabsModel[] | null>(null);
  const [browsing, setBrowsing] = useState(false);
  const [chosen, setChosen] = useState<ChosenVoice | null>(null);
  const [previewError, setPreviewError] = useState("");
  const checkSequence = useRef(0);
  const keySet = Boolean(voice.elevenlabs_key_set);
  const voiceID = voice.elevenlabs_voice_id ?? "";
  const modelID = voice.elevenlabs_model_id ?? "";
  const delivery: ElevenLabsDelivery = voice.elevenlabs_delivery ?? {};

  const check = useCallback(async (typed?: { api_key?: string; voice_id?: string }) => {
    const sequence = ++checkSequence.current;
    setChecking(true);
    setFailure("");
    try {
      const result = await api.elevenLabsAccount(typed);
      if (sequence !== checkSequence.current) return;
      setAccount(result);
      if (result.key_status === "ok" && !typed?.api_key) {
        try {
          const list = await api.elevenLabsModels();
          if (sequence === checkSequence.current) setModels(list.models ?? []);
        } catch {
          if (sequence === checkSequence.current) setModels(null);
        }
      }
    } catch (error) {
      if (sequence === checkSequence.current) setFailure(error instanceof Error ? translateKnown(error.message) : t("Request failed"));
    } finally {
      if (sequence === checkSequence.current) setChecking(false);
    }
  }, []);

  // The saved key is checked when the section opens and after it changes.
  useEffect(() => {
    if (keySet) void check();
    else setAccount(null);
  }, [keySet, check]);

  const status = elevenLabsStatus(account, checking, failure, keySet);
  const canBrowse = account?.permissions?.voices_read?.state === "ok";
  const speechOnly = account?.key_status === "restricted" || (account?.key_status === "ok" && !canBrowse);
  const selectedVoice = chosen?.voice_id === voiceID ? chosen : account?.selected_voice?.voice_id === voiceID ? account.selected_voice : null;
  const modelList = models?.length ? models : null;
  const modelOptions = modelList
    ? [...modelList].sort((a, b) => (MODEL_ORDER.indexOf(a.model_id) + 1 || 99) - (MODEL_ORDER.indexOf(b.model_id) + 1 || 99)).map((model) => model.model_id)
    : MODEL_ORDER;
  if (modelID && !modelOptions.includes(modelID)) modelOptions.push(modelID);
  const liveModel = modelList?.find((model) => model.model_id === modelID);
  const supports = liveModel?.supports ?? fallbackSupports(modelID);
  const modelLabel = (id: string) => {
    const known = MODEL_LABELS[id];
    if (known) return translateKnown(known);
    return modelList?.find((model) => model.model_id === id)?.name || id;
  };
  const modelHint = MODEL_HINTS[modelID];
  const languages = liveModel?.languages ?? [];

  const setDelivery = (key: DeliveryKey | "language", value: number | string | undefined) => {
    const next: ElevenLabsDelivery = { ...delivery };
    if (value === undefined || value === "") delete next[key];
    else (next as Record<string, number | string>)[key] = value;
    patch({ elevenlabs_delivery: next });
  };

  async function preview(url?: string | null) {
    if (!url) return;
    setPreviewError("");
    try {
      stopAllAudioPlayback();
      await playBlob(await api.elevenLabsPreview(url));
    } catch (error) {
      setPreviewError(error instanceof Error ? translateKnown(error.message) : t("The preview could not play."));
    }
  }

  const choose = (picked: ChosenVoice) => {
    setChosen(picked);
    patch({ elevenlabs_voice_id: picked.voice_id });
    setBrowsing(false);
  };

  return (
    <div className="elevenlabs-settings">
      <div className="elevenlabs-status voice-inset-note" role="status" aria-live="polite">
        <span className="status-readout">
          <span className="status-dot" data-state={status.dot} aria-hidden="true" />
          <strong>{status.label}</strong>
          {status.detail && <span className="elevenlabs-status-detail">{status.detail}</span>}
        </span>
        {keySet && <button type="button" className="btn btn-quiet btn-sm" disabled={locked || checking} onClick={() => void check()}>{t("Check again")}</button>}
      </div>

      <div className="field elevenlabs-key-field">
        <div className="elevenlabs-key-head">
          <label className="label" htmlFor={`${baseID}-key`}>{t("API key")}{keySet && <span className="badge">{t("set")}</span>}</label>
          <a href={KEY_PAGE} target="_blank" rel="noreferrer noopener">{t("Create a key at ElevenLabs")}</a>
        </div>
        <div className="field-action-row">
          <input id={`${baseID}-key`} type="password" autoComplete="off" placeholder={keySet ? t("set (leave blank to keep)") : t("Paste API key")} value={newKey} disabled={locked} onChange={(event) => setNewKey(event.target.value)} />
          <button type="button" className="btn btn-secondary" disabled={locked || checking || (!keySet && !newKey.trim())} onClick={() => void check({ api_key: newKey.trim() || undefined, voice_id: voiceID || undefined })}>{t("Check key")}</button>
        </div>
        <span className="hint">{t("Stored on this computer and never shown again. Checking reads your plan and voices; it doesn't spend credits. A key limited to Text to Speech still speaks; add Voices (read) to browse voices here.")}</span>
      </div>
      <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(event) => setClearKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Clear API key on save")}</span></label>

      <div className="field elevenlabs-voice-field">
        <span className="label" id={`${baseID}-voice`}>{t("Voice")}</span>
        {canBrowse && !speechOnly ? (
          <div className="elevenlabs-voice-row" aria-labelledby={`${baseID}-voice`}>
            <button type="button" className="btn btn-secondary elevenlabs-play" disabled={!selectedVoice?.preview_url} aria-label={t("Play a preview of {name}", { name: selectedVoice?.name ?? voiceID })} onClick={() => void preview(selectedVoice?.preview_url)}>
              <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><path d="M3.5 2.2v9.6L11.5 7z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" /></svg>
            </button>
            <div className="elevenlabs-voice-summary">
              <span className="elevenlabs-voice-name">{selectedVoice?.name ?? voiceID}</span>
              <span className="chip-row">{voiceLabels(selectedVoice?.labels).map((label) => <span key={label} className="badge">{label}</span>)}</span>
            </div>
            <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => setBrowsing(true)}>{t("Change voice…")}</button>
          </div>
        ) : (
          <input aria-labelledby={`${baseID}-voice`} className="mono-input" type="text" value={voiceID} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} />
        )}
        {account?.selected_voice_problem && voiceID === (account.selected_voice?.voice_id ?? voiceID) && !chosen && (
          <span className="voice-status-error">{t("This voice isn't in your ElevenLabs account. Choose another voice.")}</span>
        )}
        {previewError && <span className="voice-status-error" role="alert">{previewError}</span>}
        {speechOnly && <span className="hint">{t("Copy the voice ID from the voice's page in ElevenLabs.")}</span>}
      </div>

      <div className="field">
        <label className="label" htmlFor={`${baseID}-model`}>{t("Model")}</label>
        <select id={`${baseID}-model`} value={modelID} disabled={locked} onChange={(event) => patch({ elevenlabs_model_id: event.target.value })}>
          {modelOptions.map((id) => <option key={id} value={id}>{modelLabel(id)}</option>)}
        </select>
        {modelHint && <span className="hint">{translateKnown(modelHint)}</span>}
        {account?.selected_model_problem && <span className="voice-status-error">{t("ElevenLabs doesn't offer this model to your account. Choose another model.")}</span>}
      </div>

      <details className="advanced-fields elevenlabs-delivery">
        <summary>{t("Delivery")}</summary>
        {DELIVERY.filter((item) => supports[item.key]).map((item) => {
          const value = delivery[item.key];
          const id = `${baseID}-${item.key}`;
          return (
            <div className="field elevenlabs-delivery-row" key={item.key}>
              <label className="label" htmlFor={id}>{translateKnown(item.label)}</label>
              <div className="elevenlabs-range">
                <RangeInput id={id} min={item.min} max={item.max} step={item.step} value={value ?? item.neutral} disabled={locked}
                  aria-valuetext={value === undefined ? t("Voice setting") : formatNumber(value)}
                  onChange={(event) => setDelivery(item.key, Number(event.target.value))} />
                <span className="elevenlabs-range-value">{value === undefined ? t("Voice setting") : formatNumber(value)}</span>
                {value !== undefined && <button type="button" className="btn btn-quiet btn-sm" disabled={locked} onClick={() => setDelivery(item.key, undefined)}>{t("Reset")}</button>}
              </div>
              <span className="hint">{translateKnown(item.hint)}</span>
            </div>
          );
        })}
        {DELIVERY.some((item) => !supports[item.key]) && (
          <p className="hint">{t("Controls this model ignores are hidden, and settings it ignores are never sent.")}</p>
        )}
        {supports.language_code && (
          <div className="field">
            <label className="label" htmlFor={`${baseID}-language`}>{t("Language")}</label>
            <select id={`${baseID}-language`} value={delivery.language ?? ""} disabled={locked} onChange={(event) => setDelivery("language", event.target.value)}>
              <option value="">{t("Detect automatically")}</option>
              {languages.map((language) => <option key={language.language_id} value={language.language_id}>{language.name}</option>)}
              {delivery.language && !languages.some((language) => language.language_id === delivery.language) && <option value={delivery.language}>{delivery.language}</option>}
            </select>
            <span className="hint">{t("Helps short replies keep the right accent.")}</span>
          </div>
        )}
        {canBrowse && !speechOnly && (
          <div className="field">
            <label className="label" htmlFor={`${baseID}-voice-id`}>{t("Voice ID")}</label>
            <input id={`${baseID}-voice-id`} className="mono-input" type="text" value={voiceID} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} />
          </div>
        )}
      </details>

      <p className="voice-inset-note elevenlabs-cloud-note">
        <strong>{t("Cloud speech.")}</strong>{" "}
        {t("Spoken replies are sent to ElevenLabs and follow ElevenLabs' Prohibited Use Policy. Voice Library voices that screen what they say are hidden by default. For speech that never leaves this computer, use Faster Qwen3-TTS or Chatterbox.")}
      </p>

      {browsing && (
        <ElevenLabsVoiceBrowser
          currentVoiceID={voiceID}
          libraryViaAPI={account?.library_via_api}
          canAddVoices={account?.key_status === "ok"}
          onClose={() => setBrowsing(false)}
          onChoose={choose}
          onPreview={preview}
        />
      )}
    </div>
  );
}
