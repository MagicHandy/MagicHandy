// Settings as a routed workspace (not an overlay). Sub-sections are real hash
// routes: #/settings/device|model|prompts|diagnostics. Device/model/diagnostics
// share one Save; prompt sets, memory, reset use their own immediate APIs.
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings, SettingsUpdate } from "../api/types";
import { BluetoothBridge } from "../components/BluetoothBridge";
import { DiagnosticsPanel } from "../components/DiagnosticsPanel";
import { MemoryManager } from "../components/MemoryManager";
import { PromptSetEditor } from "../components/PromptSetEditor";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useHashRoute, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");
const SECTIONS = [
  { id: "device", label: "Device" },
  { id: "model", label: "Model" },
  { id: "prompts", label: "Prompts & memory" },
  { id: "diagnostics", label: "Diagnostics" },
] as const;

export function SettingsRoute() {
  const { backendOnline, readOnly, state, refresh } = useAppState();
  const { show } = useToast();
  const hash = useHashRoute();
  const requestedSection = hash.split("/")[2] || "device";
  const section = SECTIONS.some((item) => item.id === requestedSection) ? requestedSection : "device";
  const [s, setS] = useState<PublicSettings | null>(null);
  const [newKey, setNewKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const locked = !backendOnline || readOnly;

  async function load() {
    try {
      const res = await api.getSettings();
      setS(res.settings);
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
  function patchMotion(p: Partial<PublicSettings["motion"]>) {
    setS((cur) => (cur ? { ...cur, motion: { ...cur.motion, ...p } } : cur));
  }

  async function save() {
    if (!s) return;
    const update: SettingsUpdate = {
      server: { port: s.server.port },
      device: {
        hsp_dispatch_owner: s.device.hsp_dispatch_owner,
        firmware_api_requirement: s.device.firmware_api_requirement,
        api_application_id_source: s.device.api_application_id_source,
        api_application_id_override: s.device.api_application_id_override ?? "",
        ...(newKey.trim() ? { handy_connection_key: newKey } : {}),
      },
      motion: s.motion,
      llm: s.llm,
      diagnostics: s.diagnostics,
      clear_connection_key: clearKey,
    };
    try {
      await api.saveSettings(update);
      setNewKey("");
      setClearKey(false);
      show("Settings saved.");
      refresh();
      await load();
    } catch (e) {
      show(msg(e), "error");
    }
  }

  async function checkConnection() {
    if (!s) return;
    const owner = s.device.hsp_dispatch_owner.toLowerCase().includes("blue") ? "bluetooth" : "cloud";
    try {
      await api.connectionCheck(owner);
      show(`Connection check (${owner}) reachable.`);
    } catch (e) {
      show(msg(e), "error");
    }
  }
  async function llm(action: "load" | "unload") {
    try {
      await (action === "load" ? api.llmLoad() : api.llmUnload());
      show(action === "load" ? "Model load requested." : "Model unloaded.");
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
    motion_generation_modes: [],
    llm_providers: [],
    llama_cpp_modes: [],
    prompt_sets: [],
  };
  const sel = (value: string, onChange: (v: string) => void, options: string[] = []) => (
    <select value={value} disabled={locked} onChange={(e) => onChange(e.target.value)}>
      {(options.length ? options : [value]).map((o) => (
        <option key={o} value={o}>{o}</option>
      ))}
    </select>
  );

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
            <label className="field"><span className="label">HSP dispatch owner</span>{sel(s.device.hsp_dispatch_owner, (v) => patchDevice({ hsp_dispatch_owner: v }), opt.hsp_dispatch_owners)}</label>
            <label className="field"><span className="label">Firmware / API requirement</span><input type="text" value={s.device.firmware_api_requirement} readOnly /></label>
            <label className="field"><span className="label">API application ID source</span>{sel(s.device.api_application_id_source, (v) => patchDevice({ api_application_id_source: v }), opt.api_application_id_sources)}</label>
            <label className="field"><span className="label">Developer application ID</span><input type="text" value={s.device.api_application_id_override ?? ""} disabled={locked} onChange={(e) => patchDevice({ api_application_id_override: e.target.value })} /></label>
            <label className="field"><span className="label">Handy connection key {s.device.connection_key_set && <span className="badge">set</span>}</span><input type="password" autoComplete="off" placeholder={s.device.connection_key_set ? "set (leave blank to keep)" : "Paste key"} value={newKey} disabled={locked} onChange={(e) => setNewKey(e.target.value)} /></label>
            <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked} onChange={(e) => setClearKey(e.target.checked)} /><span className="track" aria-hidden="true" /></span><span>Clear connection key on save</span></label>
            <BluetoothBridge visible={s.device.hsp_dispatch_owner.toLowerCase().includes("blue")} locked={locked} backendOnline={backendOnline} initial={state?.bluetooth_bridge} />
            <label className="field"><span className="label">Server port</span><input type="number" min={1} max={65535} value={s.server.port} disabled={locked} onChange={(e) => setS((cur) => (cur ? { ...cur, server: { port: Number(e.target.value) } } : cur))} /></label>
            <label className="field">
              <span className="label">Hardware safety lock (30ms)</span>
              <label className="toggle-line hint-block">
                <span className="toggle">
                  <input
                    type="checkbox"
                    checked={s.motion.hardware_safety_lock !== false}
                    disabled={locked}
                    onChange={(e) => patchMotion({ hardware_safety_lock: e.target.checked })}
                  />
                  <span className="track" aria-hidden="true" />
                </span>
                <span>Enable hardware safety lock</span>
              </label>
            </label>
            <p className="hint-block">Disabling the lock allows extreme chaotic motion (funscript-level density) with zero artificial latency, but may cause disconnections from network/device overload depending on your hardware limits.</p>
            <div className="row-actions"><button type="button" className="btn btn-secondary" onClick={() => void checkConnection()} disabled={locked}>Check connection</button></div>
          </>
        )}

        {section === "model" && (
          <>
            <h2 className="section-title">Local LLM</h2>
            <label className="field"><span className="label">Provider</span>{sel(s.llm.provider, (v) => patchLLM({ provider: v }), opt.llm_providers)}</label>
            <label className="field"><span className="label">llama.cpp mode</span>{sel(s.llm.llama_cpp_mode, (v) => patchLLM({ llama_cpp_mode: v }), opt.llama_cpp_modes)}</label>
            <label className="field"><span className="label">Model</span><input type="text" value={s.llm.model} disabled={locked} onChange={(e) => patchLLM({ model: e.target.value })} /></label>
            <label className="field"><span className="label">llama.cpp URL</span><input type="text" value={s.llm.llama_cpp_base_url} disabled={locked} onChange={(e) => patchLLM({ llama_cpp_base_url: e.target.value })} /></label>
            <label className="field"><span className="label">llama-server path</span><input type="text" value={s.llm.llama_cpp_runner_path ?? ""} disabled={locked} onChange={(e) => patchLLM({ llama_cpp_runner_path: e.target.value })} /></label>
            <label className="field"><span className="label">GGUF model path</span><input type="text" value={s.llm.llama_cpp_model_path ?? ""} disabled={locked} onChange={(e) => patchLLM({ llama_cpp_model_path: e.target.value })} /></label>
            <label className="field"><span className="label">Ollama URL</span><input type="text" value={s.llm.ollama_base_url} disabled={locked} onChange={(e) => patchLLM({ ollama_base_url: e.target.value })} /></label>
            <label className="field"><span className="label">Timeout ms</span><input type="number" min={1000} max={300000} value={s.llm.request_timeout_ms} disabled={locked} onChange={(e) => patchLLM({ request_timeout_ms: Number(e.target.value) })} /></label>
            <div className="row-actions"><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void llm("load")}>Load</button><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void llm("unload")}>Unload</button></div>
          </>
        )}

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
