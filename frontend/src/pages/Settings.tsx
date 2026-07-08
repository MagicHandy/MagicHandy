import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { AppSettings } from "../api/types";
import type { ConfigSectionId } from "../config/configNav";
import { UiCheckbox } from "../components/UiCheckbox";
import { useToast } from "../contexts/ToastContext";

type SettingsSection = Extract<
  ConfigSectionId,
  "session" | "motion" | "connections" | "voice" | "logs"
>;

const PHASE_IDS = [
  "intro",
  "warmup",
  "build_up",
  "active",
  "peak",
  "recovery",
  "cooldown",
] as const;

const PLANNER_WEIGHT_KEYS = [
  "tag_match",
  "intensity_fit",
  "persona_match",
  "user_rating",
  "success_score",
] as const;

export function SettingsPanel({ section }: { section: SettingsSection }) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [settings, setSettings] = useState<AppSettings | null>(null);

  useEffect(() => {
    api
      .getSettings()
      .then(setSettings)
      .catch((e) => notify(e instanceof Error ? e.message : t("common.error"), "error"));
  }, [notify, t]);

  if (!settings) return <p className="hint center">{t("common.loading")}</p>;

  const num = (v: unknown, fallback: number) => (typeof v === "number" ? v : fallback);
  const str = (v: unknown, fallback: string) => (typeof v === "string" ? v : fallback);
  const bool = (v: unknown, fallback: boolean) => (typeof v === "boolean" ? v : fallback);

  const app = (settings.app ?? {}) as Record<string, unknown>;
  const autospeak = (settings.autospeak ?? {}) as Record<string, unknown>;
  const motion = (settings.motion ?? {}) as Record<string, unknown>;
  const planner = (settings.planner ?? {}) as Record<string, unknown>;
  const plannerWeights = (planner.block_selector_weights ?? {}) as Record<string, number>;
  const plannerNum = (key: string, fallback: number) => num(planner[key], fallback);
  const safety = (settings.safety ?? {}) as Record<string, unknown>;
  const queue = (settings.queue ?? {}) as Record<string, number>;
  const ollama = (settings.ollama ?? {}) as Record<string, string>;
  const llm = (settings.llm ?? {}) as Record<string, string>;
  const llmProvider = str(llm.provider, "llama_cpp");
  const llmIsOllama = llmProvider === "ollama";
  const llmDefaultURL = llmIsOllama ? "http://127.0.0.1:11434" : "http://127.0.0.1:18080";
  const llmDefaultModel = llmIsOllama ? "llama3.2" : str(llm.model, "");
  const intiface = (settings.intiface ?? {}) as Record<string, unknown>;
  const sync = (settings.sync ?? {}) as Record<string, unknown>;
  const handy = (settings.handy ?? {}) as Record<string, string>;
  const voice = (settings.voice ?? {}) as Record<string, unknown>;
  const diagnostics = (settings.diagnostics ?? {}) as Record<string, unknown>;
  const scene = (settings.scene ?? {}) as Record<string, unknown>;
  const scenePhases = (scene.phases ?? {}) as Record<string, Record<string, unknown>>;

  const updateSection = (key: keyof AppSettings, patch: Record<string, unknown>) => {
    setSettings({
      ...settings,
      [key]: { ...(settings[key] as Record<string, unknown>), ...patch },
    });
  };

  const save = async () => {
    try {
      await api.saveSettings({
        motion: settings.motion,
        safety: settings.safety,
        queue: settings.queue,
        scene: settings.scene,
        ollama: settings.ollama,
        intiface: settings.intiface,
        handy: settings.handy,
        app: settings.app,
        planner: settings.planner,
        sync: settings.sync,
        voice: settings.voice,
        autospeak: settings.autospeak,
      });
      notify(t("config.settings.saved"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const stopWordsStr = Array.isArray(safety.stop_words)
    ? (safety.stop_words as string[]).join(", ")
    : "";
  const limitsEnabled = safety.limits_enabled !== false;

  return (
    <div className="settings-panel">
      {section === "session" && (
        <>
          <section className="glass settings-card">
            <h3>{t("config.settings.session.autospeak.title")}</h3>
            <p className="hint">{t("config.settings.session.autospeak.hint")}</p>
            <label className="check-label">
              <input
                type="checkbox"
                checked={autospeak.enabled === true}
                onChange={(e) => updateSection("autospeak", { enabled: e.target.checked })}
              />
              {t("config.settings.session.autospeak.enabled")}
            </label>
            <label className="field">
              <span>{t("config.settings.session.autospeak.minSeconds")}</span>
              <input
                type="number"
                min={0}
                max={300}
                step={1}
                value={num(autospeak.min_seconds as number, 12)}
                onChange={(e) =>
                  updateSection("autospeak", { min_seconds: Number(e.target.value) })
                }
              />
            </label>
            <label className="field">
              <span>{t("config.settings.session.autospeak.maxSeconds")}</span>
              <input
                type="number"
                min={0}
                max={300}
                step={1}
                value={num(autospeak.max_seconds as number, 45)}
                onChange={(e) =>
                  updateSection("autospeak", { max_seconds: Number(e.target.value) })
                }
              />
            </label>
            <label className="field">
              <span>{t("config.settings.session.autospeak.motionAutonomy")}</span>
              <select
                value={str(autospeak.motion_autonomy as string, "full")}
                onChange={(e) =>
                  updateSection("autospeak", { motion_autonomy: e.target.value })
                }
              >
                <option value="chat_only">{t("config.settings.session.autospeak.chatOnly")}</option>
                <option value="style">{t("config.settings.session.autospeak.styleLight")}</option>
                <option value="full">{t("config.settings.session.autospeak.fullMotion")}</option>
              </select>
            </label>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.session.appLegacy.title")}</h3>
            <label className="check-label">
              <input
                type="checkbox"
                checked={app.wait_for_user_message_before_auto_speech !== false}
                onChange={(e) =>
                  updateSection("app", {
                    wait_for_user_message_before_auto_speech: e.target.checked,
                  })
                }
              />
              {t("config.settings.session.appLegacy.waitFirstMsg")}
            </label>
            <p className="hint">{t("config.settings.session.appLegacy.waitHint")}</p>
            <label className="field">
              <span>{t("config.settings.session.appLegacy.fixedInterval")}</span>
              <input
                type="number"
                min={8}
                value={num(app.auto_message_interval_sec as number, 32)}
                onChange={(e) =>
                  updateSection("app", {
                    auto_message_interval_sec: Number(e.target.value),
                  })
                }
              />
            </label>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.session.scene.title")}</h3>
            <label className="check-label">
              <input
                type="checkbox"
                checked={scene.ai_controls_pacing !== false}
                onChange={(e) =>
                  updateSection("scene", { ai_controls_pacing: e.target.checked })
                }
              />
              {t("config.settings.session.scene.aiPacing")}
            </label>
            <p className="hint">{t("config.settings.session.scene.aiPacingHint")}</p>
            <label className="check-label">
              <input
                type="checkbox"
                checked={scene.advance_on_assistant_turn === true}
                onChange={(e) =>
                  updateSection("scene", {
                    advance_on_assistant_turn: e.target.checked,
                  })
                }
              />
              {t("config.settings.session.scene.advanceLegacy")}
            </label>
            <p className="hint section-label">{t("config.settings.session.scene.phaseMinMax")}</p>
            <div className="form-grid two">
              {PHASE_IDS.map((phaseId) => (
                <label key={phaseId} className="field">
                  <span>{t(`config.settings.session.phases.${phaseId}`)}</span>
                  <div className="inline-range">
                    <input
                      type="number"
                      min={0}
                      placeholder={t("common.min")}
                      value={num(scenePhases[phaseId]?.min_sec as number, 0)}
                      onChange={(e) => {
                        const phases = { ...scenePhases };
                        phases[phaseId] = {
                          ...(phases[phaseId] ?? {}),
                          min_sec: Number(e.target.value),
                        };
                        updateSection("scene", { ...scene, phases });
                      }}
                    />
                    <input
                      type="number"
                      min={0}
                      placeholder={t("common.max")}
                      value={num(scenePhases[phaseId]?.max_sec as number, 0)}
                      onChange={(e) => {
                        const phases = { ...scenePhases };
                        phases[phaseId] = {
                          ...(phases[phaseId] ?? {}),
                          max_sec: Number(e.target.value),
                        };
                        updateSection("scene", { ...scene, phases });
                      }}
                    />
                  </div>
                </label>
              ))}
            </div>
          </section>
        </>
      )}

      {section === "motion" && (
        <>
          <section className="glass settings-card">
            <h3>{t("config.settings.motion.title")}</h3>
            <p className="hint">{t("config.settings.motion.presetsHint")}</p>
            <div className="btn-row">
              {(["slow", "medium", "fast"] as const).map((preset) => (
                <button
                  key={preset}
                  type="button"
                  className="btn btn-sm btn-ghost"
                  onClick={async () => {
                    try {
                      await api.applyMotionPreset(preset);
                      const fresh = await api.getSettings();
                      setSettings(fresh);
                      notify(t("config.settings.motion.presetApplied", { preset }), "ok");
                    } catch (e) {
                      notify(e instanceof Error ? e.message : t("common.error"), "error");
                    }
                  }}
                >
                  {t(`patterns.speed.${preset}`)}
                </button>
              ))}
            </div>
            <div className="form-grid three">
              <label className="field">
                <span>{t("config.settings.motion.minPosition")}</span>
                <input
                  type="number"
                  value={num(motion.min_position, 10)}
                  onChange={(e) =>
                    updateSection("motion", { min_position: Number(e.target.value) })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.maxPosition")}</span>
                <input
                  type="number"
                  value={num(motion.max_position, 90)}
                  onChange={(e) =>
                    updateSection("motion", { max_position: Number(e.target.value) })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.defaultIntensity")}</span>
                <input
                  type="number"
                  value={num(motion.default_intensity, 50)}
                  onChange={(e) =>
                    updateSection("motion", {
                      default_intensity: Number(e.target.value),
                    })
                  }
                />
              </label>
            </div>
            <label className="check-label">
              <input
                type="checkbox"
                checked={motion.hardware_safety_lock !== false}
                onChange={(e) =>
                  updateSection("motion", {
                    hardware_safety_lock: e.target.checked,
                  })
                }
              />
              {t("config.settings.motion.hardwareSafetyLock")}
            </label>
            <p className="hint">{t("config.settings.motion.hardwareSafetyLockHint")}</p>
            <div className="form-grid three">
              <label className="field">
                <span>{t("config.settings.motion.maxEnqueueDuration")}</span>
                <input
                  type="number"
                  min={4}
                  value={num(motion.max_enqueue_duration_sec, 10)}
                  onChange={(e) =>
                    updateSection("motion", {
                      max_enqueue_duration_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.smallPatternMax")}</span>
                <input
                  type="number"
                  step={0.5}
                  value={num(motion.small_pattern_max_sec, 4)}
                  onChange={(e) =>
                    updateSection("motion", {
                      small_pattern_max_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.smallLoopMax")}</span>
                <input
                  type="number"
                  value={num(motion.small_pattern_loop_max_sec, 40)}
                  onChange={(e) =>
                    updateSection("motion", {
                      small_pattern_loop_max_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.maxFullScript")}</span>
                <input
                  type="number"
                  min={30}
                  value={num(motion.max_full_script_duration_sec as number, 600)}
                  onChange={(e) =>
                    updateSection("motion", {
                      max_full_script_duration_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
            </div>
            <label className="field">
              <span>{t("config.settings.motion.targetBpm")}</span>
              <input
                type="number"
                value={plannerNum("target_bpm_hint", 75)}
                onChange={(e) =>
                  updateSection("planner", {
                    target_bpm_hint: Number(e.target.value),
                  })
                }
              />
            </label>
            <h4 className="settings-sub">{t("config.settings.motion.weightsTitle")}</h4>
            <p className="hint">{t("config.settings.motion.weightsHint")}</p>
            <div className="form-grid two">
              {PLANNER_WEIGHT_KEYS.map((key) => (
                <label key={key} className="field">
                  <span>{t(`config.settings.motion.weights.${key}`)}</span>
                  <input
                    type="number"
                    step={0.05}
                    min={0}
                    max={1}
                    value={num(plannerWeights[key], 0.1)}
                    onChange={(e) =>
                      updateSection("planner", {
                        block_selector_weights: {
                          ...plannerWeights,
                          [key]: Number(e.target.value),
                        },
                      })
                    }
                  />
                </label>
              ))}
            </div>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.motion.safety.title")}</h3>
            <label className="check-label">
              <input
                type="checkbox"
                checked={limitsEnabled}
                onChange={(e) =>
                  updateSection("safety", { limits_enabled: e.target.checked })
                }
              />
              {t("config.settings.motion.safety.limitsEnabled")}
            </label>
            <p className="hint">{t("config.settings.motion.safety.limitsHint")}</p>
            <div className="form-grid two">
              <label className="field">
                <span>{t("config.settings.motion.safety.maxIntensity")}</span>
                <input
                  type="number"
                  disabled={!limitsEnabled}
                  value={num(safety.max_intensity as number, 85)}
                  onChange={(e) =>
                    updateSection("safety", { max_intensity: Number(e.target.value) })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.safety.maxSpeed")}</span>
                <input
                  type="number"
                  disabled={!limitsEnabled}
                  value={num(safety.max_speed_units_per_sec as number, 180)}
                  onChange={(e) =>
                    updateSection("safety", {
                      max_speed_units_per_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
            </div>
            <label className="field">
              <span>{t("config.settings.motion.safety.stopWords")}</span>
              <input
                value={stopWordsStr}
                onChange={(e) =>
                  updateSection("safety", {
                    stop_words: e.target.value
                      .split(",")
                      .map((w) => w.trim())
                      .filter(Boolean),
                  })
                }
              />
            </label>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.motion.queue.title")}</h3>
            <div className="form-grid two">
              <label className="field">
                <span>{t("config.settings.motion.queue.targetBuffer")}</span>
                <input
                  type="number"
                  value={num(queue.target_buffer_sec, 45)}
                  onChange={(e) =>
                    updateSection("queue", {
                      target_buffer_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.motion.queue.minBuffer")}</span>
                <input
                  type="number"
                  value={num(queue.min_buffer_sec, 10)}
                  onChange={(e) =>
                    updateSection("queue", { min_buffer_sec: Number(e.target.value) })
                  }
                />
              </label>
            </div>
          </section>
        </>
      )}

      {section === "connections" && (
        <>
          <section className="glass settings-card">
            <h3>
              {llmIsOllama
                ? t("config.settings.connections.ollama.title")
                : t("config.settings.connections.llamaCpp.title")}
            </h3>
            {!llmIsOllama && llm.llama_cpp_mode === "managed" && (
              <p className="hint">{t("config.settings.connections.llamaCpp.managedHint")}</p>
            )}
            <label className="field">
              <span>
                {llmIsOllama
                  ? t("config.settings.connections.ollama.url")
                  : t("config.settings.connections.llamaCpp.url")}
              </span>
              <input
                value={str(ollama.base_url, llmDefaultURL)}
                onChange={(e) => updateSection("ollama", { base_url: e.target.value })}
              />
            </label>
            <label className="field">
              <span>
                {llmIsOllama
                  ? t("config.settings.connections.ollama.model")
                  : t("config.settings.connections.llamaCpp.model")}
              </span>
              <input
                value={str(ollama.model, llmDefaultModel)}
                onChange={(e) => updateSection("ollama", { model: e.target.value })}
              />
            </label>
            <label className="field">
              <span>{t("config.settings.connections.ollama.responseLimit")}</span>
              <input
                type="number"
                min={80}
                max={1024}
                value={str(ollama.num_predict, "320")}
                onChange={(e) =>
                  updateSection("ollama", { num_predict: Number(e.target.value) })
                }
              />
            </label>
            <label className="field">
              <span>{t("config.settings.connections.ollama.maxChatChars")}</span>
              <input
                type="number"
                min={80}
                max={800}
                value={str(ollama.max_message_chars, "280")}
                onChange={(e) =>
                  updateSection("ollama", {
                    max_message_chars: Number(e.target.value),
                  })
                }
              />
            </label>
            <p className="hint">{t("config.settings.connections.ollama.verboseHint")}</p>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.connections.handy.title")}</h3>
            <label className="field">
              <span>{t("config.settings.connections.handy.defaultMode")}</span>
              <select
                value={str(handy.transport, "handy_cloud")}
                onChange={(e) => updateSection("handy", { transport: e.target.value })}
              >
                <option value="intiface">{t("device.intifaceLocal")}</option>
                <option value="handy_cloud">{t("device.handyKeyApi")}</option>
              </select>
            </label>
            <label className="field">
              <span>{t("config.settings.connections.handy.connectionKey")}</span>
              <input
                type="password"
                value={str(handy.connection_key, "")}
                onChange={(e) =>
                  updateSection("handy", { connection_key: e.target.value })
                }
                placeholder={t("config.settings.connections.handy.keyPlaceholder")}
                autoComplete="off"
              />
            </label>
            <label className="field">
              <span>{t("config.settings.connections.handy.apiBaseUrl")}</span>
              <input
                value={str(handy.base_url, "https://www.handyfeeling.com/api/handy/v2/")}
                onChange={(e) => updateSection("handy", { base_url: e.target.value })}
              />
            </label>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.connections.intiface.title")}</h3>
            <label className="field">
              <span>{t("config.settings.connections.intiface.wsUrl")}</span>
              <input
                value={str(intiface.server_url as string, "ws://127.0.0.1:12345")}
                onChange={(e) =>
                  updateSection("intiface", { server_url: e.target.value })
                }
              />
            </label>
            <label className="field">
              <span>{t("config.settings.connections.intiface.preferredDevice")}</span>
              <input
                value={str(intiface.preferred_device_name as string, "The Handy")}
                onChange={(e) =>
                  updateSection("intiface", {
                    preferred_device_name: e.target.value,
                  })
                }
              />
            </label>
            <div className="form-grid two">
              <label className="field field-check">
                <UiCheckbox
                  label={t("config.settings.connections.intiface.autoConnect")}
                  checked={bool(intiface.auto_connect, true)}
                  onChange={(e) =>
                    updateSection("intiface", { auto_connect: e.target.checked })
                  }
                />
              </label>
              <label className="field field-check">
                <UiCheckbox
                  label={t("config.settings.connections.intiface.autoScan")}
                  checked={bool(intiface.auto_scan, true)}
                  onChange={(e) =>
                    updateSection("intiface", { auto_scan: e.target.checked })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.connections.intiface.connectionTimeout")}</span>
                <input
                  type="number"
                  value={num(intiface.connection_timeout_sec as number, 30)}
                  onChange={(e) =>
                    updateSection("intiface", {
                      connection_timeout_sec: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="field">
                <span>{t("config.settings.connections.intiface.sendLead")}</span>
                <input
                  type="number"
                  value={num(intiface.send_lead_ms as number, 75)}
                  onChange={(e) =>
                    updateSection("intiface", { send_lead_ms: Number(e.target.value) })
                  }
                />
              </label>
            </div>
          </section>

          <section className="glass settings-card">
            <h3>{t("config.settings.connections.sync.title")}</h3>
            <div className="form-grid two">
              <label className="field">
                <span>{t("config.settings.connections.sync.offset")}</span>
                <input
                  type="number"
                  value={num(sync.offset_ms as number, -160)}
                  onChange={(e) =>
                    updateSection("sync", { offset_ms: Number(e.target.value) })
                  }
                />
              </label>
              <label className="field field-check">
                <UiCheckbox
                  label={t("config.settings.connections.sync.autoSyncOnConnect")}
                  checked={bool(sync.auto_sync_on_connect, true)}
                  onChange={(e) =>
                    updateSection("sync", { auto_sync_on_connect: e.target.checked })
                  }
                />
              </label>
            </div>
          </section>
        </>
      )}

      {section === "voice" && (
        <>
          <section className="glass settings-card">
            <h3>{t("config.settings.voice.tts.title")}</h3>
            <p className="hint">{t("config.settings.voice.tts.installHint")}</p>
            <label className="field row-check">
              <input
                type="checkbox"
                checked={Boolean(voice.enabled)}
                onChange={(e) => updateSection("voice", { enabled: e.target.checked })}
              />
              <span>{t("config.settings.voice.tts.enableTts")}</span>
            </label>
            <label className="field row-check">
              <input
                type="checkbox"
                checked={Boolean(voice.auto_speak_after_chat)}
                onChange={(e) =>
                  updateSection("voice", {
                    auto_speak_after_chat: e.target.checked,
                  })
                }
              />
              <span>{t("config.settings.voice.tts.autoSpeak")}</span>
            </label>
            <label className="field">
              <span>{t("config.settings.voice.tts.voiceId")}</span>
              <input
                value={str(voice.voice_id, "pt-BR-FranciscaNeural")}
                onChange={(e) => updateSection("voice", { voice_id: e.target.value })}
              />
            </label>
            <h4 className="settings-sub">{t("config.settings.voice.stt.title")}</h4>
            <label className="field row-check">
              <input
                type="checkbox"
                checked={Boolean(voice.stt_enabled)}
                onChange={(e) =>
                  updateSection("voice", { stt_enabled: e.target.checked })
                }
              />
              <span>{t("config.settings.voice.stt.enableStt")}</span>
            </label>
            <label className="field">
              <span>{t("config.settings.voice.stt.provider")}</span>
              <select
                value={str(voice.stt_provider, "faster_whisper")}
                onChange={(e) =>
                  updateSection("voice", { stt_provider: e.target.value })
                }
              >
                <option value="faster_whisper">{t("config.settings.voice.stt.local")}</option>
                <option value="browser">{t("config.settings.voice.stt.browser")}</option>
              </select>
            </label>
            <label className="field row-check">
              <input
                type="checkbox"
                checked={Boolean(voice.stt_auto_send)}
                onChange={(e) =>
                  updateSection("voice", { stt_auto_send: e.target.checked })
                }
              />
              <span>{t("config.settings.voice.stt.autoSend")}</span>
            </label>
            <label className="field">
              <span>{t("config.settings.voice.stt.model")}</span>
              <select
                value={str(voice.stt_model, "base")}
                onChange={(e) => updateSection("voice", { stt_model: e.target.value })}
              >
                <option value="tiny">{t("config.settings.voice.stt.tiny")}</option>
                <option value="base">{t("config.settings.voice.stt.base")}</option>
                <option value="small">{t("config.settings.voice.stt.small")}</option>
              </select>
            </label>
            <label className="field">
              <span>{t("config.settings.voice.stt.language")}</span>
              <input
                value={str(voice.stt_language, "pt")}
                onChange={(e) =>
                  updateSection("voice", { stt_language: e.target.value })
                }
                placeholder={t("config.settings.voice.stt.langPlaceholder")}
              />
            </label>
          </section>
        </>
      )}

      {section === "logs" && (
        <>
          <section className="glass settings-card">
            <h3>{t("config.settings.logs.title")}</h3>
            <label className="check-label">
              <input
                type="checkbox"
                checked={diagnostics.log_handy_motion !== false}
                onChange={(e) =>
                  updateSection("diagnostics", { log_handy_motion: e.target.checked })
                }
              />
              {t("config.settings.logs.motionLog")}
            </label>
            <label className="check-label">
              <input
                type="checkbox"
                checked={diagnostics.log_handy_motion_verbose === true}
                onChange={(e) =>
                  updateSection("diagnostics", {
                    log_handy_motion_verbose: e.target.checked,
                  })
                }
              />
              {t("config.settings.logs.verboseLog")}
            </label>
            <p className="hint">{t("config.settings.logs.verboseHint")}</p>
          </section>
        </>
      )}

      <div className="settings-save-bar glass">
        <p className="hint">{t("config.settings.saveBarHint")}</p>
        <button type="button" className="btn btn-primary" onClick={save}>
          {t("config.settings.saveSettings")}
        </button>
      </div>
    </div>
  );
}

export function SettingsRawPanel() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [raw, setRaw] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setSettings(s);
        setRaw(JSON.stringify(s, null, 2));
      })
      .catch((e) => notify(e instanceof Error ? e.message : t("common.error"), "error"));
  }, [notify, t]);

  const apply = () => {
    try {
      const parsed = JSON.parse(raw) as AppSettings;
      setSettings(parsed);
      setError(null);
      notify(t("config.settings.raw.applied"), "ok");
    } catch {
      setError(t("config.settings.raw.invalid"));
    }
  };

  const save = async () => {
    try {
      const parsed = JSON.parse(raw) as AppSettings;
      await api.saveSettings(parsed);
      setSettings(parsed);
      setRaw(JSON.stringify(parsed, null, 2));
      notify(t("config.settings.saved"), "ok");
    } catch (e) {
      notify(
        e instanceof SyntaxError
          ? t("config.settings.raw.invalidBeforeSave")
          : e instanceof Error
            ? e.message
            : t("common.error"),
        "error",
      );
    }
  };

  if (!settings) return <p className="hint center">{t("common.loading")}</p>;

  return (
    <div className="settings-raw-panel">
      <section className="glass settings-card">
        <h3>{t("config.settings.raw.title")}</h3>
        <p className="hint">{t("config.settings.raw.hint")}</p>
        <textarea
          className="mono json-raw-area json-raw-area--full"
          rows={24}
          value={raw}
          onChange={(e) => {
            setRaw(e.target.value);
            setError(null);
          }}
          spellCheck={false}
        />
        {error && <p className="hint json-raw-error">{error}</p>}
        <div className="btn-row">
          <button type="button" className="btn btn-ghost" onClick={apply}>
            {t("config.settings.raw.applyJson")}
          </button>
          <button type="button" className="btn btn-primary" onClick={save}>
            {t("config.settings.raw.saveServer")}
          </button>
        </div>
      </section>
    </div>
  );
}
