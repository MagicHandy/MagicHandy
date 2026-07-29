// Settings as a routed workspace (not an overlay). Sub-sections are real hash
// routes: #/settings/device|media|model|prompts|diagnostics. Routed sections
// share one Save; prompt sets, memory, reset use their own immediate APIs.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings, SettingsUpdate } from "../api/types";
import { DiagnosticsPanel } from "../components/DiagnosticsPanel";
import { ManualMotionTest } from "../components/ManualMotionTest";
import { MediaSettingsPanel } from "../components/MediaSettingsPanel";
import { MemoryManager } from "../components/MemoryManager";
import { ModelSettingsPanel } from "../components/ModelSettingsPanel";
import { PromptSetEditor } from "../components/PromptSetEditor";
import { VoiceSettingsPanel } from "../components/VoiceSettingsPanel";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { LOCALE_OPTIONS, normalizeLocale, t, translateKnown, type MessageKey } from "../i18n";
import { useAppState, useHashRoute, useToast } from "../state/app-state";
import type { MediaSettingsPayload } from "../api/types";

const msg = (e: unknown) => (e instanceof Error ? translateKnown(e.message) : t("Request failed"));
const firmwareRequirementLabel = (value: string) => value === "firmware_v4_api_v3_required"
  ? t("Cloud REST requires Handy firmware v4 with API v3 access.")
  : value;
const CHAT_VOICE_LABELS: Partial<Record<string, MessageKey>> = {
  utility: "Utility (neutral assistant)",
  warm: "Warm (flirtatious, never explicit)",
  intimate: "Intimate (sensual partner)",
  explicit: "Explicit (direct sexual language)",
};
const USER_ANATOMY_LABELS: Partial<Record<string, MessageKey>> = {
  penis: "Penis",
  vagina: "Vagina / vulva",
  custom: "Custom wording",
};
const SETTING_OPTION_LABELS: Partial<Record<string, MessageKey>> = {
  cloud_rest: "Cloud REST",
  browser_bluetooth: "Browser Bluetooth",
  intiface: "Intiface Central",
  bundled: "Bundled application ID",
  bundled_app_id: "Bundled application ID",
  developer_override: "Developer override",
  normal: "Normal",
  debug: "Debug",
  trace: "Trace",
};

const optionLabel = (labels: Partial<Record<string, MessageKey>>, value: string) => {
  const label = labels[value];
  return label ? translateKnown(label) : value;
};
const MAX_CUSTOM_ANATOMY_CHARS = 120;
const MAX_PERSONA_DESCRIPTION_CHARS = 500;
const clampCharacters = (value: string, limit: number) => Array.from(value).slice(0, limit).join("");
const PROMPT_SET_LABELS: Record<string, string> = {
  magichandy_motion_v1: "English",
  magichandy_motion_v1_es: "Español",
  magichandy_motion_v1_pt_br: "Português (Brasil)",
  magichandy_motion_v1_zh_hans: "简体中文",
  magichandy_motion_v1_ja: "日本語",
};
const SECTIONS = [
  { id: "general", label: "General" },
  { id: "device", label: "Device" },
  { id: "media", label: "Media library" },
  { id: "model", label: "Model" },
  { id: "chat", label: "Chat" },
  { id: "voice", label: "Voice" },
  { id: "prompts", label: "Prompts & memory" },
  { id: "diagnostics", label: "Diagnostics" },
] as const;

export function SettingsRoute() {
  const { backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const hash = useHashRoute();
  const requestedSection = hash.split("/")[2] || "general";
  const section = SECTIONS.some((item) => item.id === requestedSection) ? requestedSection : "general";
  const [s, setS] = useState<PublicSettings | null>(null);
  // The last-saved snapshot, kept to detect unsaved voice edits: worker
  // controls act on the saved config and lock while the form is dirty.
  const [saved, setSaved] = useState<PublicSettings | null>(null);
  const [newKey, setNewKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const [newElevenLabsKey, setNewElevenLabsKey] = useState("");
  const [clearElevenLabsKey, setClearElevenLabsKey] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [saving, setSaving] = useState(false);
  const mounted = useRef(true);
  const loadGeneration = useRef(0);
  const savingRef = useRef(false);
  const locked = !backendOnline || readOnly || loading;

  async function load() {
    if (!mounted.current) return;
    const generation = ++loadGeneration.current;
    setLoading(true);
    setLoadError("");
    try {
      const res = await api.getSettings();
      if (!mounted.current || generation !== loadGeneration.current) return;
      setS(res.settings);
      setSaved(res.settings);
    } catch (e) {
      if (mounted.current && generation === loadGeneration.current) setLoadError(msg(e));
    } finally {
      if (mounted.current && generation === loadGeneration.current) setLoading(false);
    }
  }
  useEffect(() => {
    mounted.current = true;
    void load();
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
    };
  }, []);

  function patchUI(p: Partial<NonNullable<PublicSettings["ui"]>>) {
    setS((cur) => (cur ? { ...cur, ui: { locale: cur.ui?.locale ?? "en", ...p } } : cur));
  }
  function patchDevice(p: Partial<PublicSettings["device"]>) {
    setS((cur) => (cur ? { ...cur, device: { ...cur.device, ...p } } : cur));
  }
  function patchLLM(p: Partial<PublicSettings["llm"]>) {
    setS((cur) => (cur ? { ...cur, llm: { ...cur.llm, ...p } } : cur));
  }
  // Spread rather than rebuilding the object field by field: this section owns
  // a dozen settings now, and an enumerated rebuild silently drops whichever
  // one a later change forgets to list.
  function patchMedia(p: Partial<MediaSettingsPayload>) {
    setS((cur) => (cur
      ? { ...cur, media: { ...cur.media, library_paths: cur.media?.library_paths ?? [], ...p } }
      : cur));
  }
  function patchMotion(p: Partial<PublicSettings["motion"]>) {
    setS((cur) => (cur ? { ...cur, motion: { ...cur.motion, ...p } } : cur));
  }
  function patchVoice(p: Partial<PublicSettings["voice"]>) {
    setS((cur) => (cur ? { ...cur, voice: { ...cur.voice, ...p } } : cur));
  }
  function patchChat(p: Partial<NonNullable<PublicSettings["chat"]>>) {
    setS((cur) => (cur ? {
      ...cur,
      chat: {
        startup_behavior: cur.chat?.startup_behavior ?? "previous",
        keep_unsaved_on_exit: cur.chat?.keep_unsaved_on_exit ?? false,
        ...p,
      },
    } : cur));
  }

  async function save() {
    if (!s || savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    const connectionKey = newKey.trim();
    const elevenLabsKey = newElevenLabsKey.trim();
    const update: SettingsUpdate = {
      server: { port: s.server.port },
      ui: { locale: normalizeLocale(s.ui?.locale) },
      // Playback filters save through their immediate endpoint. Omitting them
      // here prevents a stale Settings draft from overwriting newer values.
      // Everything else in this section has no immediate endpoint, so it has to
      // be listed here or the form would appear to save and change nothing.
      media: {
        library_paths: s.media?.library_paths ?? [],
        auto_scan_on_startup: s.media?.auto_scan_on_startup ?? false,
        remove_missing_on_scan: s.media?.remove_missing_on_scan ?? true,
        script_offset_ms: s.media?.script_offset_ms ?? 0,
        ffmpeg_path: s.media?.ffmpeg_path ?? "",
        convert_h265_for_compatibility: s.media?.convert_h265_for_compatibility ?? false,
        reencode_codec: s.media?.reencode_codec ?? "h264",
        reencode_crf_h264: s.media?.reencode_crf_h264 ?? 23,
        reencode_crf_h265: s.media?.reencode_crf_h265 ?? 28,
        reencode_preset: s.media?.reencode_preset ?? "medium",
        reencode_audio_kbps: s.media?.reencode_audio_kbps ?? 192,
        generate_thumbnails_on_scan: s.media?.generate_thumbnails_on_scan ?? false,
        convert_incompatible_on_scan: s.media?.convert_incompatible_on_scan ?? false,
        show_superseded_originals: s.media?.show_superseded_originals ?? false,
      },
      device: {
        hsp_dispatch_owner: s.device.hsp_dispatch_owner,
        intiface_server_address: s.device.intiface_server_address,
        firmware_api_requirement: s.device.firmware_api_requirement,
        api_application_id_source: s.device.api_application_id_source,
        api_application_id_override: s.device.api_application_id_override ?? "",
        ...(connectionKey ? { handy_connection_key: connectionKey } : {}),
      },
      motion: s.motion,
      llm: s.llm,
      // Exact write shape: the ElevenLabs key is write-only (sent only when
      // newly typed); elevenlabs_key_set never goes back to the server.
      voice: {
        enabled: s.voice?.enabled ?? false,
        tts_provider: s.voice?.tts_provider ?? "none",
        asr_provider: s.voice?.asr_provider ?? "none",
        tts_worker_path: s.voice?.tts_worker_path ?? "",
        tts_worker_args: s.voice?.tts_worker_args ?? [],
        asr_worker_path: s.voice?.asr_worker_path ?? "",
        asr_worker_args: s.voice?.asr_worker_args ?? [],
        speak_replies: s.voice?.speak_replies ?? false,
        elevenlabs_voice_id: s.voice?.elevenlabs_voice_id ?? "",
        elevenlabs_model_id: s.voice?.elevenlabs_model_id ?? "",
        parakeet_server_path: s.voice?.parakeet_server_path ?? "",
        parakeet_model_path: s.voice?.parakeet_model_path ?? "",
        parakeet_port: s.voice?.parakeet_port ?? 8990,
        parakeet_source: s.voice?.parakeet_source ?? "app_managed",
        asr_base_url: s.voice?.asr_base_url ?? "",
        asr_model: s.voice?.asr_model ?? "",
        input_mode: s.voice?.input_mode ?? "hands_free",
        input_sensitivity: s.voice?.input_sensitivity ?? 55,
        input_silence_ms: s.voice?.input_silence_ms ?? 900,
        input_noise_suppression: s.voice?.input_noise_suppression ?? true,
        neutts_runner_path: s.voice?.neutts_runner_path ?? "",
        neutts_reference_wav: s.voice?.neutts_reference_wav ?? "",
        neutts_reference_codes: s.voice?.neutts_reference_codes ?? "",
        neutts_reference_text: s.voice?.neutts_reference_text ?? "",
        neutts_backbone: s.voice?.neutts_backbone ?? "",
        neutts_sampling_mode: s.voice?.neutts_sampling_mode ?? "fixed",
        neutts_sampler_seed: s.voice?.neutts_sampler_seed ?? 3,
        ...(elevenLabsKey ? { elevenlabs_api_key: elevenLabsKey } : {}),
        clear_elevenlabs_key: elevenLabsKey ? false : clearElevenLabsKey,
      },
      chat: {
        startup_behavior: s.chat?.startup_behavior ?? "previous",
        keep_unsaved_on_exit: s.chat?.keep_unsaved_on_exit ?? false,
      },
      diagnostics: s.diagnostics,
      clear_connection_key: connectionKey ? false : clearKey,
    };
    try {
      await api.saveSettings(update);
      show(t("Settings saved."));
      refresh();
      if (mounted.current) {
        setNewKey("");
        setClearKey(false);
        setNewElevenLabsKey("");
        setClearElevenLabsKey(false);
        await load();
      }
    } catch (e) {
      show(msg(e), "error");
    } finally {
      savingRef.current = false;
      if (mounted.current) setSaving(false);
    }
  }

  function applyReset(settings: PublicSettings) {
    loadGeneration.current += 1;
    setS(settings);
    setSaved(settings);
    setNewKey("");
    setClearKey(false);
    setNewElevenLabsKey("");
    setClearElevenLabsKey(false);
  }

  if (!s) return (
    <>
      <WorkspaceHead title={t("Settings")} />
      {loadError ? (
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Settings unavailable")}</h2>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button>
        </div>
      ) : (
        <p className="form-status" role="status">{loading ? t("Loading settings…") : t("Settings unavailable.")}</p>
      )}
    </>
  );

  const opt = s.options ?? {
    hsp_dispatch_owners: [],
    api_application_id_sources: [],
    diagnostics_verbosities: [],
    motion_styles: [],
    llm_providers: [],
    llama_cpp_modes: [],
    llm_reasoning_modes: [],
    llm_max_output_tokens: [],
    llm_chat_voices: [],
    llm_user_anatomies: [],
    prompt_sets: [],
    tts_providers: [],
    asr_providers: [],
    parakeet_sources: [],
    neutts_sampling_modes: [],
    chat_startup_behaviors: [],
    locales: [],
  };
  const sel = (value: string, onChange: (v: string) => void, options: string[] = []) => (
    <select value={value} disabled={locked} onChange={(e) => onChange(e.target.value)}>
      {(options.length ? options : [value]).map((o) => (
        <option key={o} value={o}>{optionLabel(SETTING_OPTION_LABELS, o)}</option>
      ))}
    </select>
  );
  const owner = s.device.hsp_dispatch_owner;

  return (
    <>
      <WorkspaceHead title={t("Settings")} />
      <nav className="settings-nav" aria-label={t("Settings sections")}>
        {SECTIONS.map((sec) => (
          <a key={sec.id} href={`#/settings/${sec.id}`} aria-current={section === sec.id ? "page" : undefined}>{translateKnown(sec.label)}</a>
        ))}
      </nav>

      {loadError && (
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Settings refresh failed")}</h2>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button>
        </div>
      )}
      {loading && <p className="form-status" role="status">{t("Refreshing settings…")}</p>}

      <section className="panel">
        {section === "general" && (
          <>
            <h2 className="section-title">{t("General")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Interface")}</h3>
              <label className="field">
                <span className="label">{t("Language")}</span>
                <select
                  aria-label={t("Language")}
                  value={normalizeLocale(s.ui?.locale)}
                  disabled={locked}
                  onChange={(event) => patchUI({ locale: event.target.value })}
                >
                  {LOCALE_OPTIONS.filter((locale) => !(opt.locales?.length) || opt.locales.includes(locale.value)).map((locale) => (
                    <option key={locale.value} value={locale.value}>{locale.label}</option>
                  ))}
                </select>
                <span className="hint-block">{t("The saved language is shared by every open tab and applies after Save settings.")}</span>
              </label>
            </div>
          </>
        )}

        {section === "device" && (
          <>
            <h2 className="section-title">{t("Device")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Connection")}</h3>
              <label className="field"><span className="label">{t("Dispatch owner")}</span>{sel(owner, (v) => patchDevice({ hsp_dispatch_owner: v }), opt.hsp_dispatch_owners)}</label>
              {owner === "cloud_rest" && <>
                <div className="device-requirement" role="note" aria-labelledby="device-firmware-requirement">
                  <span id="device-firmware-requirement" className="label">{t("Firmware / API requirement")}</span>
                  <p>{firmwareRequirementLabel(s.device.firmware_api_requirement)}</p>
                </div>
                <label className="field"><span className="label">{t("API application ID source")}</span>{sel(s.device.api_application_id_source, (v) => patchDevice({ api_application_id_source: v }), opt.api_application_id_sources)}</label>
                {s.device.api_application_id_source === "developer_override" && <label className="field"><span className="label">{t("Developer application ID")}</span><input type="text" value={s.device.api_application_id_override ?? ""} disabled={locked} onChange={(e) => patchDevice({ api_application_id_override: e.target.value })} /></label>}
                <label className="field"><span className="label">{t("Handy connection key")}{s.device.connection_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={s.device.connection_key_set ? t("set (leave blank to keep)") : t("Paste key")} value={newKey} disabled={locked} onChange={(e) => { setNewKey(e.target.value); if (e.target.value.trim()) setClearKey(false); }} /></label>
                <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(e) => { setClearKey(e.target.checked); if (e.target.checked) setNewKey(""); }} /><span className="track" aria-hidden="true" /></span><span>{t("Clear connection key on save")}</span></label>
              </>}
              {owner === "intiface" && <>
                <label className="field"><span className="label">{t("Intiface Central server")}</span><input type="url" value={s.device.intiface_server_address} disabled={locked} spellCheck={false} onChange={(e) => patchDevice({ intiface_server_address: e.target.value })} /></label>
              </>}
            </div>
            <div className="group">
              <h3 className="group-title">{t("Local server")}</h3>
              <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={s.server.port} disabled={locked} onChange={(e) => setS((cur) => (cur ? { ...cur, server: { port: Number(e.target.value) } } : cur))} /></label>
            </div>
            <ManualMotionTest />
          </>
        )}

        {section === "model" && (
          <ModelSettingsPanel
            settings={s.llm}
            saved={saved?.llm}
            providers={opt.llm_providers ?? []}
            llamaModes={opt.llama_cpp_modes ?? []}
            reasoningModes={opt.llm_reasoning_modes ?? []}
            maxOutputOptions={opt.llm_max_output_tokens ?? []}
            locked={locked}
            patch={patchLLM}
          />
        )}

        {section === "media" && (
          <>
            <h2 className="section-title">{t("Media library")}</h2>
            <MediaSettingsPanel
              media={s.media ?? { library_paths: [] }}
              savedMedia={saved?.media ?? { library_paths: [] }}
              limitVideoScriptSpeed={s.motion.apply_video_speed_limit ?? false}
              onLimitVideoScriptSpeedChange={(enabled) => patchMotion({ apply_video_speed_limit: enabled })}
              locked={locked}
              onChange={patchMedia}
            />
          </>
        )}

        {section === "chat" && (
          <>
            <h2 className="section-title">{t("Chat")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Sessions")}</h3>
              <label className="field">
                <span className="label">{t("When MagicHandy starts")}</span>
                <select
                  value={s.chat?.startup_behavior ?? "previous"}
                  disabled={locked}
                  onChange={(event) => patchChat({
                    startup_behavior: event.target.value,
                    ...(event.target.value === "new" ? { keep_unsaved_on_exit: false } : {}),
                  })}
                >
                  {(opt.chat_startup_behaviors ?? ["previous", "new"]).map((behavior) => (
                    <option key={behavior} value={behavior}>{behavior === "new" ? t("Start a new chat") : t("Open the previous chat")}</option>
                  ))}
                </select>
                <span className="hint-block">{t("Previous restores the last retained chat. New creates a blank, unsaved tab on every launch.")}</span>
              </label>
              <label className="toggle-line">
                <span className="toggle">
                  <input
                    type="checkbox"
                    checked={s.chat?.keep_unsaved_on_exit ?? false}
                    disabled={locked || s.chat?.startup_behavior === "new"}
                    onChange={(event) => patchChat({ keep_unsaved_on_exit: event.target.checked })}
                  />
                  <span className="track" aria-hidden="true" />
                </span>
                <span>{t("Keep an unsaved current chat after closing MagicHandy")}<small>{s.chat?.startup_behavior === "new"
                    ? t("Starting with a new chat always removes the prior unsaved draft.")
                    : t("Off by default. Saved tabs are always kept; use Save chat from the tab menu or its right-click menu.")}</small>
                </span>
              </label>
            </div>
          </>
        )}

        {section === "voice" && <VoiceSettingsPanel
          settings={s}
          locked={locked}
          dirty={JSON.stringify(s.voice) !== JSON.stringify(saved?.voice) || Boolean(newElevenLabsKey.trim()) || clearElevenLabsKey}
          patch={patchVoice}
          newKey={newElevenLabsKey}
          setNewKey={(value) => { setNewElevenLabsKey(value); if (value.trim()) setClearElevenLabsKey(false); }}
          clearKey={clearElevenLabsKey}
          setClearKey={(value) => { setClearElevenLabsKey(value); if (value) setNewElevenLabsKey(""); }}
        />}

        {section === "prompts" && (
          <>
            <h2 className="section-title">{t("Prompts & memory")}</h2>
            <div className="group">
            <h3 className="group-title">{t("Reply style")}</h3>
            <label className="field">
              <span className="label">{t("Active prompt set")}<span className="hint-inline">{t("saved with Save settings")}</span></span>
              <select value={s.llm.prompt_set} disabled={locked} onChange={(event) => patchLLM({ prompt_set: event.target.value })}>
                {(opt.prompt_sets?.length ? opt.prompt_sets : [s.llm.prompt_set]).map((promptSet) => (
                  <option key={promptSet} value={promptSet}>{PROMPT_SET_LABELS[promptSet] ?? promptSet}</option>
                ))}
              </select>
              <span className="hint-block">{t("Built-in prompt sets choose the default language for model replies. Custom prompt sets keep their own instructions.")}</span>
            </label>
            <label className="field">
              <span className="label">{t("Chat voice")}<span className="hint-inline">{t("how sexual the model's replies may be")}</span></span>
              <select value={s.llm.chat_voice ?? "utility"} disabled={locked} onChange={(e) => patchLLM({ chat_voice: e.target.value })}>
                {(opt.llm_chat_voices?.length ? opt.llm_chat_voices : ["utility"]).map((voice) => (
                  <option key={voice} value={voice}>{optionLabel(CHAT_VOICE_LABELS, voice)}</option>
                ))}
              </select>
            </label>
            <p className="hint">{t("Utility keeps the neutral assistant register. Warm is flirtatious but never explicit. Intimate speaks as a partner with sensual language. Explicit permits direct sexual language like the legacy app. Voice changes wording only; motion limits, capability gates, and Stop are identical at every level.")}</p>
            </div>
            <div className="group">
            <h3 className="group-title">{t("Persona and anatomy")}</h3>
            <label className="field">
              <span className="label">{t("User anatomy")}<span className="hint-inline">{t("separate from partner persona")}</span></span>
              <select
                value={s.llm.user_anatomy ?? "penis"}
                disabled={locked}
                onChange={(event) => patchLLM({ user_anatomy: event.target.value as PublicSettings["llm"]["user_anatomy"] })}
              >
                {(opt.llm_user_anatomies?.length ? opt.llm_user_anatomies : ["penis", "vagina", "custom"]).map((anatomy) => (
                  <option key={anatomy} value={anatomy}>{optionLabel(USER_ANATOMY_LABELS, anatomy)}</option>
                ))}
              </select>
            </label>
            {(s.llm.user_anatomy ?? "penis") === "custom" && (
              <label className="field">
                <span className="label">{t("Custom anatomy wording")}<span className="hint-inline">{Array.from(s.llm.custom_anatomy ?? "").length} / {MAX_CUSTOM_ANATOMY_CHARS}</span>
                </span>
                <input
                  type="text"
                  value={s.llm.custom_anatomy ?? ""}
                  disabled={locked}
                  autoComplete="off"
                  onChange={(event) => patchLLM({ custom_anatomy: clampCharacters(event.target.value, MAX_CUSTOM_ANATOMY_CHARS) })}
                />
              </label>
            )}
            <label className="field">
              <span className="label">{t("Persona description")}<span className="hint-inline">{t("optional · {current} / {max}", { current: Array.from(s.llm.persona_description ?? "").length, max: MAX_PERSONA_DESCRIPTION_CHARS })}</span>
              </span>
              <textarea
                rows={3}
                value={s.llm.persona_description ?? ""}
                disabled={locked}
                onChange={(event) => patchLLM({ persona_description: clampCharacters(event.target.value, MAX_PERSONA_DESCRIPTION_CHARS) })}
              />
            </label>
            <p className="hint">{t("Anatomy context and persona apply to interactive Warm, Intimate, and Explicit replies. Direct anatomy wording is reserved for Explicit; the other levels keep references indirect. This bounded context cannot change motion permissions or limits.")}</p>
            </div>
            <PromptSetEditor locked={locked} />
            <MemoryManager locked={locked} />
          </>
        )}

        {section === "diagnostics" && (
          <>
            <h2 className="section-title">{t("Diagnostics")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Logging")}</h3>
              <label className="field"><span className="label">{t("Diagnostics verbosity")}</span>{sel(s.diagnostics.verbosity, (v) => setS((cur) => (cur ? { ...cur, diagnostics: { verbosity: v } } : cur)), opt.diagnostics_verbosities)}</label>
              <p className="hint-block">{t("Higher levels record more detail in the trace export. They do not change what the report below shows.")}</p>
            </div>
            <DiagnosticsPanel locked={locked} onReset={applyReset} />
          </>
        )}

        <div className="row-actions settings-actions">
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={locked || saving}>{saving ? t("Saving settings") : t("Save settings")}</button>
          {locked && <span className="form-status">{loading ? t("Refreshing settings") : backendOnline ? t("Read-only client") : t("Core offline")}</span>}
        </div>
      </section>
    </>
  );
}
