// Settings as a routed workspace (not an overlay). Sub-sections are real hash
// routes: #/settings/device|media|chat|diagnostics. Routed sections
// share one Save; prompt sets, memory, reset use their own immediate APIs.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { NotificationCategory, PublicSettings, SettingsUpdate } from "../api/types";
import { DiagnosticsPanel } from "../components/DiagnosticsPanel";
import { ManualMotionTest } from "../components/ManualMotionTest";
import { MediaSettingsPanel } from "../components/MediaSettingsPanel";
import { VoiceSettingsPanel } from "../components/VoiceSettingsPanel";
import { rebaseSettingsDraft } from "../util/settings-draft";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { ThemePicker } from "../components/ThemePicker";
import { UpdateSettingsPanel } from "../components/UpdateSettingsPanel";
import { RuntimeUpdateSettings } from "../components/RuntimeUpdateSettings";
import { LOCALE_OPTIONS, normalizeLocale, t, translateKnown, type MessageKey } from "../i18n";
import { useAppState, useHashRoute, useToast } from "../state/app-state";
import type { MediaSettingsPayload } from "../api/types";
import { DEFAULT_THEME, normalizeTheme } from "../theme";
import { notificationCategories } from "../notification-preferences";
import { AccountSettingsPanel } from "../components/AccountSettingsPanel";
import { ChatSettingsPanel, resolveChatSettingsSection } from "../components/ChatSettingsPanel";
import { SettingsNavigation, SettingsNavigationLabel } from "../components/SettingsNavigation";
import { LabsSettings } from "../components/LabsSettings";
import { DismissibleNotice } from "../components/DismissibleNotice";
import { CoyoteSettings, DEFAULT_ESTIM_SETTINGS } from "../components/CoyoteSettings";
import type { DeviceId, EstimSettings, VibrationSettings } from "../api/types";
import { DEFAULT_VIBRATION_SETTINGS, VibratorSettings } from "../components/VibratorSettings";
import { DEVICE_ORDER, configuredDevices, playbackDevices } from "../api/devices";

const msg = (e: unknown) => (e instanceof Error ? translateKnown(e.message) : t("Request failed"));
const firmwareRequirementLabel = (value: string) => value === "firmware_v4_api_v3_required"
  ? t("Cloud REST requires Handy firmware v4 with API v3 access.")
  : value;
const SETTING_OPTION_LABELS: Partial<Record<string, MessageKey>> = {
  cloud_rest: "Cloud REST",
  browser_bluetooth: "Browser Bluetooth",
  native_bluetooth: "Native Bluetooth",
  intiface: "Intiface Central",
  bundled: "Bundled application ID",
  bundled_app_id: "Bundled application ID",
  developer_override: "Developer override",
  normal: "Normal",
  debug: "Debug",
  trace: "Trace",
};

// The settings payload marks a remembered device with a flag; the write shape
// carries only forget_device, and the server keeps the address.
function stripRemembered(estim: PublicSettings["estim"]): Partial<EstimSettings> {
  if (!estim) return {};
  const { remembered_device: _remembered, ...rest } = estim;
  return rest;
}

const optionLabel = (labels: Partial<Record<string, MessageKey>>, value: string) => {
  const label = labels[value];
  return label ? translateKnown(label) : value;
};
function notificationPreferenceOptions(): Array<{ category: NotificationCategory; label: string; detail: string }> {
  return [
    { category: "app", label: t("Routine app feedback"), detail: t("Save confirmations and other short-lived command results.") },
    { category: "system", label: t("Core and device status"), detail: t("Connection loss, recovery, and other operational state changes.") },
    { category: "library", label: t("Library and media tasks"), detail: t("Completed scans, thumbnail jobs, and media conversions.") },
    { category: "voice", label: t("Voice worker alerts"), detail: t("Speech input and output failures that need attention.") },
    { category: "updates", label: t("Software updates"), detail: t("A new stable MagicHandy release is available.") },
  ];
}
const SECTIONS = [
  { id: "general", label: "General" },
  { id: "access", label: "Access" },
  { id: "device", label: "Device" },
  { id: "media", label: "Media library", compact: "Media" },
  { id: "chat", label: "Chat" },
  { id: "voice", label: "Voice" },
  { id: "diagnostics", label: "Diagnostics" },
] as const;

function SettingsSections({ section, accessOnly = false }: { section: string; accessOnly?: boolean }) {
  return <SettingsNavigation className="settings-nav" label={t("Settings sections")} current={section}>
    {(accessOnly ? SECTIONS.filter(item => item.id === "access") : SECTIONS).map(item =>
      <a key={item.id} href={`#/settings/${item.id}`} aria-label={translateKnown(item.label)} title={translateKnown(item.label)} aria-current={section === item.id ? "page" : undefined}><SettingsNavigationLabel label={item.label} compact={"compact" in item ? item.compact : undefined} /></a>)}
  </SettingsNavigation>;
}

export function SettingsRoute() {
  const { backendOnline, readOnly, refresh, state } = useAppState();
  const { show } = useToast();
  const hash = useHashRoute();
  const requestedSection = hash.split("/")[2] || "general";
  // Existing model/prompt bookmarks retain their destination within Chat.
  const legacyChatSection = requestedSection === "model" || requestedSection === "prompts";
  const requestedRoot = legacyChatSection ? "chat" : requestedSection;
  const section = SECTIONS.some((item) => item.id === requestedRoot) ? requestedRoot : "general";
  const chatSection = resolveChatSettingsSection(legacyChatSection ? requestedSection : hash.split("/")[3] || "conversation");
  const restricted = state?.capabilities?.configure_host === false;
  const onlyAccess = restricted || section === "access";
  const [s, setS] = useState<PublicSettings | null>(null);
  // The last-saved snapshot, kept to detect unsaved voice edits: worker
  // controls act on the saved config and lock while the form is dirty.
  const [saved, setSaved] = useState<PublicSettings | null>(null);
  const [newKey, setNewKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const [newElevenLabsKey, setNewElevenLabsKey] = useState("");
  const [clearElevenLabsKey, setClearElevenLabsKey] = useState(false);
  const [newOpenAITTSKey, setNewOpenAITTSKey] = useState("");
  const [clearOpenAITTSKey, setClearOpenAITTSKey] = useState(false);
  const [forgetCoyote, setForgetCoyote] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [saving, setSaving] = useState(false);
  const mounted = useRef(true);
  const loadGeneration = useRef(0);
  const savingRef = useRef(false);
  const savedRef = useRef(saved);
  savedRef.current = saved;
  const locked = !backendOnline || readOnly || loading || state?.capabilities?.configure_host === false;
  const liveDevice = state?.settings?.device;
  const liveDeviceKey = JSON.stringify(liveDevice);
  const previousDeviceKey = useRef(liveDeviceKey);

  useEffect(() => {
    if (previousDeviceKey.current === liveDeviceKey) return;
    previousDeviceKey.current = liveDeviceKey;
    const baseline = savedRef.current;
    if (!liveDevice || !baseline) return;
    // The manager applies these fields immediately. Follow its confirmed
    // changes while retaining unrelated edits, including the failure policy.
    const next = { ...baseline, device: liveDevice };
    setS(draft => draft ? {
      ...rebaseSettingsDraft(baseline, draft, next),
      device: {
        ...rebaseSettingsDraft(baseline.device, draft.device, liveDevice),
        configured_devices: liveDevice.configured_devices,
        playback_devices: liveDevice.playback_devices,
        output: liveDevice.output,
        hsp_dispatch_owner: liveDevice.hsp_dispatch_owner,
      },
    } : draft);
    setSaved(next);
  }, [liveDeviceKey]);

  async function load(preserveDraft = false) {
    if (!mounted.current) return;
    const generation = ++loadGeneration.current;
    setLoading(true);
    setLoadError("");
    try {
      const res = await api.getSettings();
      if (!mounted.current || generation !== loadGeneration.current) return;
      const baseline = savedRef.current;
      setS((draft) => preserveDraft && baseline && draft
        ? rebaseSettingsDraft(baseline, draft, res.settings)
        : res.settings);
      setSaved(res.settings);
    } catch (e) {
      if (mounted.current && generation === loadGeneration.current) setLoadError(msg(e));
    } finally {
      if (mounted.current && generation === loadGeneration.current) setLoading(false);
    }
  }
  useEffect(() => {
    mounted.current = true;
    // Account/security pages use their own bounded endpoints. They should not
    // wait for host settings, and visiting them must preserve an existing draft.
    if (!onlyAccess && !savedRef.current) void load();
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
    };
  }, [onlyAccess]);

  function patchUI(p: Partial<NonNullable<PublicSettings["ui"]>>) {
    setS((cur) => (cur ? {
      ...cur,
      ui: {
        ...cur.ui,
        locale: cur.ui?.locale ?? "en",
        theme: cur.ui?.theme ?? DEFAULT_THEME,
        setup_completed: cur.ui?.setup_completed ?? true,
        update_check_mode: cur.ui?.update_check_mode ?? "automatic",
        notification_categories: notificationCategories(cur.ui?.notification_categories),
        ...p,
      },
    } : cur));
  }
  function patchNotificationCategory(category: NotificationCategory, enabled: boolean) {
    const current = notificationCategories(s?.ui?.notification_categories);
    patchUI({
      notification_categories: enabled
        ? Array.from(new Set([...current, category]))
        : current.filter((item) => item !== category),
    });
  }
  function patchDevice(p: Partial<PublicSettings["device"]>) {
    setS((cur) => (cur ? { ...cur, device: { ...cur.device, ...p } } : cur));
  }
  function patchEstim(p: Partial<EstimSettings>) {
    setS((cur) => (cur ? { ...cur, estim: { ...DEFAULT_ESTIM_SETTINGS, ...cur.estim, ...p } } : cur));
  }
  function patchVibration(p: Partial<VibrationSettings>) {
    setS((cur) => (cur ? { ...cur, vibration: { ...DEFAULT_VIBRATION_SETTINGS, ...cur.vibration, ...p } } : cur));
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
    const openAITTSKey = newOpenAITTSKey.trim();
    const update: SettingsUpdate = {
      server: { port: s.server.port },
      ui: {
        locale: normalizeLocale(s.ui?.locale),
        theme: normalizeTheme(s.ui?.theme),
        setup_completed: s.ui?.setup_completed ?? true,
        update_check_mode: s.ui?.update_check_mode ?? "automatic",
        notification_categories: notificationCategories(s.ui?.notification_categories),
        show_remote_navigation: s.ui?.show_remote_navigation ?? true,
      },
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
        playback_devices: playbackDevices(s.device),
        linked_failure_policy: s.device.linked_failure_policy ?? "stop_all",
        hsp_dispatch_owner: s.device.hsp_dispatch_owner,
        intiface_server_address: s.device.intiface_server_address,
        firmware_api_requirement: s.device.firmware_api_requirement,
        api_application_id_source: s.device.api_application_id_source,
        api_application_id_override: s.device.api_application_id_override ?? "",
        ...(connectionKey ? { handy_connection_key: connectionKey } : {}),
      },
      estim: { ...DEFAULT_ESTIM_SETTINGS, ...stripRemembered(s.estim), forget_device: forgetCoyote },
      vibration: { ...DEFAULT_VIBRATION_SETTINGS, ...s.vibration },
      motion: s.motion,
      autopilot: s.autopilot,
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
        chat_speech_policy: s.voice?.chat_speech_policy ?? "interrupt",
        elevenlabs_voice_id: s.voice?.elevenlabs_voice_id ?? "",
        elevenlabs_model_id: s.voice?.elevenlabs_model_id ?? "",
        tts_auto_launch: s.voice?.tts_auto_launch ?? false,
        tts_base_url: s.voice?.tts_base_url ?? "http://127.0.0.1:8991",
        tts_model: s.voice?.tts_model ?? "",
        tts_voice: s.voice?.tts_voice ?? "",
        tts_response_format: s.voice?.tts_response_format ?? "wav",
        tts_health_path: s.voice?.tts_health_path ?? "/health",
        tts_module_root: s.voice?.tts_module_root ?? "",
        tts_server_port: s.voice?.tts_server_port ?? 8991,
        tts_reference_wav: s.voice?.tts_reference_wav ?? "",
        tts_reference_text: s.voice?.tts_reference_text ?? "",
        tts_language: s.voice?.tts_language ?? "Auto",
        tts_device: s.voice?.tts_device ?? "auto",
        tts_seed: s.voice?.tts_seed ?? 1337,
        tts_seed_mode: s.voice?.tts_seed_mode ?? "fixed",
        tts_tone_preset: s.voice?.tts_tone_preset ?? "natural",
        tts_tone_prompt: s.voice?.tts_tone_prompt ?? "",
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
        ...(elevenLabsKey ? { elevenlabs_api_key: elevenLabsKey } : {}),
        clear_elevenlabs_key: elevenLabsKey ? false : clearElevenLabsKey,
        ...(openAITTSKey ? { openai_tts_api_key: openAITTSKey } : {}),
        clear_openai_tts_key: openAITTSKey ? false : clearOpenAITTSKey,
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
        setNewOpenAITTSKey("");
        setClearOpenAITTSKey(false);
        setForgetCoyote(false);
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
    setNewOpenAITTSKey("");
    setClearOpenAITTSKey(false);
  }

  if (onlyAccess) return <>
    <WorkspaceHead title={t("Settings")} />
    <div className="settings-layout">
      <SettingsSections section="access" accessOnly={restricted} />
      <div className="settings-main">
        {restricted && <p className="hint-block">{t("Host settings and diagnostics are managed by an administrator.")}</p>}
        <section className="panel settings-panel access-panel"><AccountSettingsPanel backendOnline={backendOnline} /></section>
      </div>
    </div>
  </>;

  if (!s || !state) return (
    <>
      <WorkspaceHead title={t("Settings")} />
      {loadError ? (
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Settings unavailable")}</h2>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button>
        </div>
      ) : (
        <p className="form-status" role="status">{loading || !state ? t("Loading settings…") : t("Settings unavailable.")}</p>
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
    llm_managed_load_policies: [],
    llama_cpp_context_sizes: [],
    llm_reasoning_modes: [],
    llm_max_output_tokens: [],
    llm_chat_voices: [],
    llm_user_anatomies: [],
    prompt_sets: [],
    tts_providers: [],
    asr_providers: [],
    parakeet_sources: [],
    tts_devices: [],
    chat_startup_behaviors: [],
    locales: [],
    themes: [],
  };
  const sel = (value: string, onChange: (v: string) => void, options: string[] = []) => (
    <select value={value} disabled={locked} onChange={(e) => onChange(e.target.value)}>
      {(options.length ? options : [value]).map((o) => (
        <option key={o} value={o}>{optionLabel(SETTING_OPTION_LABELS, o)}</option>
      ))}
    </select>
  );
  const owner = s.device.hsp_dispatch_owner;
  const playing = playbackDevices(s.device);
  const configured = configuredDevices(s.device);
  const incompleteModel = s?.llm.connections?.some(connection => !connection.model.trim() && (connection.id === s.llm.conversation_connection_id || (s.llm.motion_planner?.provider === "connection" && connection.id === s.llm.motion_planner.connection_id))) ?? false;
  const saveActions = <div className="row-actions settings-actions">
    <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={locked || saving || incompleteModel}>{saving ? t("Saving settings") : t("Save settings")}</button>
    {incompleteModel && <p className="hint">{t("Choose a model for each selected connection before saving settings.")}</p>}
    {locked && <span className="form-status">{loading ? t("Refreshing settings") : backendOnline ? t("Read-only client") : t("Core offline")}</span>}
  </div>;

  return (
    <>
      <WorkspaceHead title={t("Settings")} />
      <div className="settings-layout">
      <SettingsSections section={section} />
      <div className="settings-main">

      {loadError && (
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Settings refresh failed")}</h2>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button>
        </div>
      )}
      {loading && <p className="form-status" role="status">{t("Refreshing settings…")}</p>}

      <section className="panel settings-panel">
        {section === "chat" && <ChatSettingsPanel section={chatSection} settings={s} saved={saved} activeLLM={state?.settings?.llm} modelRouting={state?.settings?.model_routing} options={opt} locked={locked} patchLLM={patchLLM} patchChat={patchChat} actions={saveActions} />}

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
              <ThemePicker
                collapsible
                value={s.ui?.theme}
                allowedThemes={opt.themes}
                disabled={locked}
                onChange={(theme) => patchUI({ theme })}
              />
              <label className="toggle-line">
                <span className="toggle"><input type="checkbox" checked={s.ui?.show_remote_navigation !== false} disabled={locked} onChange={event => patchUI({ show_remote_navigation: event.target.checked })} /><span className="track" aria-hidden="true" /></span>
                <span>{t("Show Remote in sidebar")}</span>
              </label>
              <p className="hint-block">{t("Hides only the shortcut. The remote remains available at its own address. Applies to every browser after Save settings.")}</p>
            <p className="hint-block"><a href="#/settings/access/profile">{t("Manage hidden informational notices")}</a></p>
            </div>
            <LabsSettings/>
            <div className="group">
              <h3 className="group-title">{t("Updates")}</h3>
              <UpdateSettingsPanel
                currentVersion={state?.version}
                automatic={(s.ui?.update_check_mode ?? "automatic") !== "manual"}
                preferenceDisabled={locked}
                checkDisabled={!backendOnline || loading}
                onAutomaticChange={(automatic) => patchUI({ update_check_mode: automatic ? "automatic" : "manual" })}
              />
              <RuntimeUpdateSettings
                automatic={(s.ui?.runtime_update_mode ?? "automatic") !== "manual"}
                preferenceDisabled={locked}
                actionDisabled={locked || !backendOnline}
                onAutomaticChange={(automatic) => patchUI({ runtime_update_mode: automatic ? "automatic" : "manual" })}
              />
            </div>
            <div className="group">
              <h3 className="group-title">{t("Notification history")}</h3>
              <p className="hint-block">{t("Choose which events create bell history and unread badges. Live activity, offline state, and Emergency Stop remain visible regardless.")}</p>
              <div className="notification-preference-list">
                {notificationPreferenceOptions().map((option) => (
                  <label className="toggle-line" key={option.category}>
                    <span className="toggle"><input type="checkbox" checked={notificationCategories(s.ui?.notification_categories).includes(option.category)} disabled={locked} onChange={(event) => patchNotificationCategory(option.category, event.target.checked)} /><span className="track" aria-hidden="true" /></span>
                    <span>{option.label}<small>{option.detail}</small></span>
                  </label>
                ))}
              </div>
            </div>
            <div className="group">
              <h3 className="group-title">{t("Guided setup")}</h3>
              <p className="hint-block">{t("Review device, model, and optional voice choices in the same guided flow used after installation.")}</p>
              <a className="btn btn-secondary settings-setup-link" href="#/setup/reconfigure">{t("Run setup again")}</a>
            </div>
          </>
        )}

        {section === "device" && (
          <>
            <h2 className="section-title">{t("Device")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Connections")}</h3>
              <button type="button" className="btn btn-secondary" onClick={() => window.dispatchEvent(new Event("magichandy:open-connections"))}>{t("Manage connections")}</button>
              <p className="hint-block">{t("Add and connect devices in the connection manager. Saved connections can stay connected without joining playback.")}</p>
              <fieldset className="field playback-devices">
                <legend className="label">{t("Playback devices")}</legend>
                {configured.length === 0 && <p className="form-status">{t("None (chat only)")}</p>}
                {configured.map((device) => (
                  <label key={device} className="toggle-line">
                    <span className="toggle">
                      <input type="checkbox" checked={playing.includes(device)} disabled={locked}
                        onChange={(event) => patchDevice({ playback_devices: withPlayback(playing, device, event.target.checked) })} />
                      <span className="track" aria-hidden="true" />
                    </span>
                    <span>{playbackDeviceName(device)}</span>
                  </label>
                ))}
              </fieldset>
              <label className="field"><span className="label">{t("On device failure")}</span>
                <select value={s.device.linked_failure_policy ?? "stop_all"} disabled={locked} onChange={(event) => patchDevice({ linked_failure_policy: event.target.value as "stop_all" | "continue_healthy" })}>
                  <option value="stop_all">{t("Stop all devices (recommended)")}</option>
                  <option value="continue_healthy">{t("Continue on the healthy device")}</option>
                </select>
              </label>
              <p className="hint-block">{t("Applies to unexpected disconnections and playback errors during a linked run. Reconnecting never rejoins a run automatically. Emergency Stop always stops every device.")}</p>
              <p className="hint-block">{t("Changing playback devices or the failure policy stops the current run.")}</p>
            </div>
            {configured.includes("coyote") && <CoyoteSettings
              estim={{ ...DEFAULT_ESTIM_SETTINGS, ...s.estim }}
              rememberedDevice={Boolean(s.estim?.remembered_device)}
              forgetDevice={forgetCoyote}
              locked={locked}
              patch={patchEstim}
              setForgetDevice={setForgetCoyote}
            />}
            {configured.includes("vibrator") && <VibratorSettings
              vibration={{ ...DEFAULT_VIBRATION_SETTINGS, ...s.vibration }}
              locked={locked}
              patch={patchVibration}
            />}
            {configured.includes("handy") && <div className="group">
              <h3 className="group-title">{t("Connection")}</h3>
              <label className="field"><span className="label">{t("Protocol")}</span>{sel(owner, (v) => patchDevice({ hsp_dispatch_owner: v }), opt.hsp_dispatch_owners)}</label>
              {owner === "cloud_rest" && <>
                <DismissibleNotice id="device-firmware" className="device-requirement">
                  <span id="device-firmware-requirement" className="label">{t("Firmware / API requirement")}</span>
                  <p>{firmwareRequirementLabel(s.device.firmware_api_requirement)}</p>
                </DismissibleNotice>
                <label className="field"><span className="label">{t("Handy connection key")}{s.device.connection_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={s.device.connection_key_set ? t("set (leave blank to keep)") : t("Paste key")} value={newKey} disabled={locked} onChange={(e) => { setNewKey(e.target.value); if (e.target.value.trim()) setClearKey(false); }} /></label>
                <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(e) => { setClearKey(e.target.checked); if (e.target.checked) setNewKey(""); }} /><span className="track" aria-hidden="true" /></span><span>{t("Clear connection key on save")}</span></label>
              </>}
              {owner === "native_bluetooth" && <p className="hint-block">{t("Uses Bluetooth on the computer running MagicHandy. Put the Handy in firmware 4 Bluetooth mode before connecting.")}</p>}
            </div>}
            {/* One Intiface Central connection serves the Handy and the vibrator,
                so its address shows whenever either uses it. */}
            {((configured.includes("handy") && owner === "intiface") || configured.includes("vibrator")) && <div className="group">
              <h3 className="group-title">{t("Intiface Central")}</h3>
              <label className="field"><span className="label">{t("Intiface Central server")}</span><input type="url" value={s.device.intiface_server_address} disabled={locked} spellCheck={false} onChange={(e) => patchDevice({ intiface_server_address: e.target.value })} /></label>
            </div>}
            <details className="group" open={s.device.api_application_id_source === "developer_override" ? true : undefined}>
              <summary className="group-title">{t("Advanced")}</summary>
              {owner === "cloud_rest" && <>
                <label className="field"><span className="label">{t("API application ID source")}</span>{sel(s.device.api_application_id_source, (v) => patchDevice({ api_application_id_source: v }), opt.api_application_id_sources)}</label>
                {s.device.api_application_id_source === "developer_override" && <label className="field"><span className="label">{t("Developer application ID")}</span><input type="text" value={s.device.api_application_id_override ?? ""} disabled={locked} onChange={(e) => patchDevice({ api_application_id_override: e.target.value })} /></label>}
              </>}
              <h3 className="group-title">{t("Local server")}</h3>
              <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={s.server.port} disabled={locked} onChange={(e) => setS((cur) => (cur ? { ...cur, server: { port: Number(e.target.value) } } : cur))} /></label>
            </details>
            {configured.includes("handy") && <ManualMotionTest />}
          </>
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

        {section === "voice" && <VoiceSettingsPanel
          settings={s}
          locked={locked}
          dirty={JSON.stringify(s.voice) !== JSON.stringify(saved?.voice) || Boolean(newElevenLabsKey.trim()) || clearElevenLabsKey || Boolean(newOpenAITTSKey.trim()) || clearOpenAITTSKey}
          patch={patchVoice}
          newKey={newElevenLabsKey}
          setNewKey={(value) => { setNewElevenLabsKey(value); if (value.trim()) setClearElevenLabsKey(false); }}
          clearKey={clearElevenLabsKey}
          setClearKey={(value) => { setClearElevenLabsKey(value); if (value) setNewElevenLabsKey(""); }}
          newOpenAIKey={newOpenAITTSKey}
          setNewOpenAIKey={(value) => { setNewOpenAITTSKey(value); if (value.trim()) setClearOpenAITTSKey(false); }}
          clearOpenAIKey={clearOpenAITTSKey}
          setClearOpenAIKey={(value) => { setClearOpenAITTSKey(value); if (value) setNewOpenAITTSKey(""); }}
          onRuntimeChanged={() => { void load(true); }}
        />}

        {section === "diagnostics" && (
          <>
            <h2 className="section-title">{t("Diagnostics")}</h2>
            <div className="group">
              <h3 className="group-title">{t("Logging")}</h3>
              <label className="field"><span className="label">{t("Diagnostics verbosity")}</span>{sel(s.diagnostics.verbosity, (v) => setS((cur) => (cur ? { ...cur, diagnostics: { verbosity: v } } : cur)), opt.diagnostics_verbosities)}</label>
              <p className="hint-block">{t("Higher levels record more runtime detail for the diagnostic report.")}</p>
            </div>
            <DiagnosticsPanel locked={locked} onReset={applyReset} />
          </>
        )}


        {section !== "chat" && saveActions}
      </section>
      </div>
      </div>
    </>
  );
}

function withPlayback(playing: DeviceId[], device: DeviceId, included: boolean): DeviceId[] {
  return DEVICE_ORDER.filter((id) => id === device ? included : playing.includes(id));
}

function playbackDeviceName(device: DeviceId): string {
  return device === "coyote" ? t("Coyote") : device === "vibrator" ? t("Vibrator") : t("The Handy");
}
