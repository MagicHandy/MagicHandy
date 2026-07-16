// Settings as a routed workspace (not an overlay). Sub-sections are real hash
// routes: #/settings/device|model|prompts|diagnostics. Device/model/diagnostics
// share one Save; prompt sets, memory, reset use their own immediate APIs.
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings, SettingsUpdate } from "../api/types";
import { DiagnosticsPanel } from "../components/DiagnosticsPanel";
import { MemoryManager } from "../components/MemoryManager";
import { ModelSettingsPanel } from "../components/ModelSettingsPanel";
import { PromptSetEditor } from "../components/PromptSetEditor";
import { VoiceSettingsPanel } from "../components/VoiceSettingsPanel";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useHashRoute, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");
const firmwareRequirementLabel = (value: string) => value === "firmware_v4_api_v3_required"
  ? "Cloud REST requires Handy firmware v4 with API v3 access."
  : value;
const SECTIONS = [
  { id: "device", label: "Device" },
  { id: "model", label: "Model" },
  { id: "voice", label: "Voice" },
  { id: "prompts", label: "Prompts & memory" },
  { id: "diagnostics", label: "Diagnostics" },
] as const;

export function SettingsRoute() {
  const { backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const hash = useHashRoute();
  const requestedSection = hash.split("/")[2] || "device";
  const section = SECTIONS.some((item) => item.id === requestedSection) ? requestedSection : "device";
  const [s, setS] = useState<PublicSettings | null>(null);
  // The last-saved snapshot, kept to detect unsaved voice edits: worker
  // controls act on the saved config and lock while the form is dirty.
  const [saved, setSaved] = useState<PublicSettings | null>(null);
  const [newKey, setNewKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const [newElevenLabsKey, setNewElevenLabsKey] = useState("");
  const [clearElevenLabsKey, setClearElevenLabsKey] = useState(false);
  const locked = !backendOnline || readOnly;

  async function load() {
    try {
      const res = await api.getSettings();
      setS(res.settings);
      setSaved(res.settings);
    } catch (e) {
      show(msg(e), "error");
    }
  }
  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function patchDevice(p: Partial<PublicSettings["device"]>) {
    setS((cur) => (cur ? { ...cur, device: { ...cur.device, ...p } } : cur));
  }
  function patchLLM(p: Partial<PublicSettings["llm"]>) {
    setS((cur) => (cur ? { ...cur, llm: { ...cur.llm, ...p } } : cur));
  }
  function patchVoice(p: Partial<PublicSettings["voice"]>) {
    setS((cur) => (cur ? { ...cur, voice: { ...cur.voice, ...p } } : cur));
  }

  async function save() {
    if (!s) return;
    const update: SettingsUpdate = {
      server: { port: s.server.port },
      device: {
        hsp_dispatch_owner: s.device.hsp_dispatch_owner,
        intiface_server_address: s.device.intiface_server_address,
        firmware_api_requirement: s.device.firmware_api_requirement,
        api_application_id_source: s.device.api_application_id_source,
        api_application_id_override: s.device.api_application_id_override ?? "",
        ...(newKey.trim() ? { handy_connection_key: newKey } : {}),
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
        ...(newElevenLabsKey.trim() ? { elevenlabs_api_key: newElevenLabsKey } : {}),
        clear_elevenlabs_key: clearElevenLabsKey,
      },
      diagnostics: s.diagnostics,
      clear_connection_key: clearKey,
    };
    try {
      await api.saveSettings(update);
      setNewKey("");
      setClearKey(false);
      setNewElevenLabsKey("");
      setClearElevenLabsKey(false);
      show("Settings saved.");
      refresh();
      await load();
    } catch (e) {
      show(msg(e), "error");
    }
  }

  if (!s) return (<><WorkspaceHead title="Settings" /><p className="form-status">Loading settings…</p></>);

  const opt = s.options ?? {
    hsp_dispatch_owners: [],
    api_application_id_sources: [],
    diagnostics_verbosities: [],
    motion_styles: [],
    llm_providers: [],
    llama_cpp_modes: [],
    llm_reasoning_modes: [],
    llm_max_output_tokens: [],
    prompt_sets: [],
    tts_providers: [],
    asr_providers: [],
    parakeet_sources: [],
    neutts_sampling_modes: [],
  };
  const sel = (value: string, onChange: (v: string) => void, options: string[] = []) => (
    <select value={value} disabled={locked} onChange={(e) => onChange(e.target.value)}>
      {(options.length ? options : [value]).map((o) => (
        <option key={o} value={o}>{o}</option>
      ))}
    </select>
  );
  const owner = s.device.hsp_dispatch_owner;

  return (
    <>
      <WorkspaceHead title="Settings" />
      <nav className="settings-nav" aria-label="Settings sections">
        {SECTIONS.map((sec) => (
          <a key={sec.id} href={`#/settings/${sec.id}`} aria-current={section === sec.id ? "page" : undefined}>{sec.label}</a>
        ))}
      </nav>

      <section className="panel">
        {section === "device" && (
          <>
            <h2 className="section-title">Device connection</h2>
            <label className="field"><span className="label">Dispatch owner</span>{sel(owner, (v) => patchDevice({ hsp_dispatch_owner: v }), opt.hsp_dispatch_owners)}</label>
            {owner === "cloud_rest" && <>
              <div className="group device-requirement" role="note" aria-labelledby="device-firmware-requirement">
                <h3 id="device-firmware-requirement" className="group-title">Firmware / API requirement</h3>
                <p>{firmwareRequirementLabel(s.device.firmware_api_requirement)}</p>
              </div>
              <label className="field"><span className="label">API application ID source</span>{sel(s.device.api_application_id_source, (v) => patchDevice({ api_application_id_source: v }), opt.api_application_id_sources)}</label>
              {s.device.api_application_id_source === "developer_override" && <label className="field"><span className="label">Developer application ID</span><input type="text" value={s.device.api_application_id_override ?? ""} disabled={locked} onChange={(e) => patchDevice({ api_application_id_override: e.target.value })} /></label>}
              <label className="field"><span className="label">Handy connection key {s.device.connection_key_set && <span className="badge">set</span>}</span><input type="password" autoComplete="off" placeholder={s.device.connection_key_set ? "set (leave blank to keep)" : "Paste key"} value={newKey} disabled={locked} onChange={(e) => setNewKey(e.target.value)} /></label>
              <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked} onChange={(e) => setClearKey(e.target.checked)} /><span className="track" aria-hidden="true" /></span><span>Clear connection key on save</span></label>
            </>}
            {owner === "intiface" && <>
              <label className="field"><span className="label">Intiface Central server</span><input type="url" value={s.device.intiface_server_address} disabled={locked} spellCheck={false} onChange={(e) => patchDevice({ intiface_server_address: e.target.value })} /></label>
            </>}
            <label className="field"><span className="label">Server port</span><input type="number" min={1} max={65535} value={s.server.port} disabled={locked} onChange={(e) => setS((cur) => (cur ? { ...cur, server: { port: Number(e.target.value) } } : cur))} /></label>
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

        {section === "voice" && <VoiceSettingsPanel
          settings={s}
          locked={locked}
          dirty={JSON.stringify(s.voice) !== JSON.stringify(saved?.voice) || Boolean(newElevenLabsKey.trim()) || clearElevenLabsKey}
          patch={patchVoice}
          newKey={newElevenLabsKey}
          setNewKey={setNewElevenLabsKey}
          clearKey={clearElevenLabsKey}
          setClearKey={setClearElevenLabsKey}
        />}

        {section === "prompts" && (
          <>
            <h2 className="section-title">Prompts &amp; memory</h2>
            <label className="field"><span className="label">Active prompt set <span className="hint-inline">saved with Save settings</span></span>{sel(s.llm.prompt_set, (v) => patchLLM({ prompt_set: v }), opt.prompt_sets)}</label>
            <div className="divider" />
            <PromptSetEditor locked={locked} />
            <div className="divider" />
            <MemoryManager locked={locked} />
          </>
        )}

        {section === "diagnostics" && (
          <>
            <h2 className="section-title">Diagnostics</h2>
            <label className="field"><span className="label">Diagnostics verbosity</span>{sel(s.diagnostics.verbosity, (v) => setS((cur) => (cur ? { ...cur, diagnostics: { verbosity: v } } : cur)), opt.diagnostics_verbosities)}</label>
            <div className="divider" />
            <DiagnosticsPanel locked={locked} />
          </>
        )}

        <div className="row-actions settings-actions">
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={locked}>Save settings</button>
          {locked && <span className="form-status">{backendOnline ? "Read-only client" : "Core offline"}</span>}
        </div>
      </section>
    </>
  );
}
