import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

const PHASES = [
  "intro",
  "warmup",
  "build_up",
  "active",
  "peak",
  "recovery",
  "cooldown",
] as const;

const ZONES = ["top", "middle", "bottom", "full", "mixed"];
const STROKES = ["short", "medium", "full"];
const SPEEDS = ["slow", "medium", "fast", "very_fast"];
const RHYTHMS = [
  "steady",
  "pulsed",
  "accelerating",
  "decelerating",
  "chaotic",
];

type PhasePrefs = Record<string, unknown>;

function parseMotion(raw: string): Record<string, PhasePrefs> {
  if (!raw.trim()) return {};
  try {
    const v = JSON.parse(raw);
    if (v && typeof v === "object" && !Array.isArray(v)) {
      return v as Record<string, PhasePrefs>;
    }
  } catch {
    /* */
  }
  return {};
}

export function PersonaMotionBiasEditor({
  valueJson,
  onChangeJson,
}: {
  valueJson: string;
  onChangeJson: (json: string) => void;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"ui" | "raw">("ui");
  const [rawDraft, setRawDraft] = useState(valueJson);
  const [rawError, setRawError] = useState<string | null>(null);
  const [openPhase, setOpenPhase] = useState<string>(PHASES[0]);

  const data = useMemo(() => parseMotion(valueJson), [valueJson]);

  useEffect(() => {
    if (mode === "raw") setRawDraft(valueJson);
  }, [valueJson, mode]);

  const patchPhase = (phase: string, patch: PhasePrefs) => {
    const next = {
      ...data,
      [phase]: { ...(data[phase] ?? {}), ...patch },
    };
    onChangeJson(JSON.stringify(next, null, 2));
  };

  const applyRaw = () => {
    const parsed = parseMotion(rawDraft);
    if (!rawDraft.trim()) {
      onChangeJson("{}");
      setRawError(null);
      return;
    }
    try {
      JSON.parse(rawDraft);
    } catch {
      setRawError(t("config.raw.invalid"));
      return;
    }
    setRawError(null);
    onChangeJson(JSON.stringify(parsed, null, 2));
  };

  const str = (phase: string, key: string) => {
    const v = data[phase]?.[key];
    return typeof v === "string" ? v : "";
  };

  const num = (phase: string, key: string) => {
    const v = data[phase]?.[key];
    return typeof v === "number" ? v : "";
  };

  return (
    <section className="json-section-editor">
      <header className="json-section-head">
        <div>
          <h4 className="json-section-title">{t("persona.motionBias.title")}</h4>
          <p className="hint json-section-desc">{t("persona.motionBias.hint")}</p>
        </div>
        <div className="view-mode-toggle" role="tablist">
          <button
            type="button"
            className={`view-mode-btn${mode === "ui" ? " active" : ""}`}
            onClick={() => setMode("ui")}
          >
            {t("persona.formMode")}
          </button>
          <button
            type="button"
            className={`view-mode-btn${mode === "raw" ? " active" : ""}`}
            onClick={() => setMode("raw")}
          >
            {t("persona.jsonMode")}
          </button>
        </div>
      </header>

      {mode === "ui" ? (
        <div className="motion-bias-phases">
          {PHASES.map((phase) => {
            const active = openPhase === phase;
            const hasData = Boolean(data[phase] && Object.keys(data[phase]).length);
            return (
              <div key={phase} className={`motion-bias-phase${active ? " motion-bias-phase--open" : ""}`}>
                <button
                  type="button"
                  className="motion-bias-phase-toggle"
                  onClick={() => setOpenPhase(active ? "" : phase)}
                >
                  <span className="motion-bias-phase-name">{phase}</span>
                  {hasData && <span className="pill pill-muted">{t("persona.configured")}</span>}
                </button>
                {active && (
                  <div className="motion-bias-phase-body form-grid two">
                    <label className="field">
                      <span>{t("persona.motionBias.zone")}</span>
                      <select
                        value={str(phase, "zone")}
                        onChange={(e) => patchPhase(phase, { zone: e.target.value })}
                      >
                        <option value="">—</option>
                        {ZONES.map((z) => (
                          <option key={z} value={z}>
                            {z}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      <span>{t("persona.motionBias.amplitude")}</span>
                      <select
                        value={str(phase, "stroke_length")}
                        onChange={(e) =>
                          patchPhase(phase, { stroke_length: e.target.value })
                        }
                      >
                        <option value="">—</option>
                        {STROKES.map((s) => (
                          <option key={s} value={s}>
                            {s}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      <span>{t("persona.motionBias.speed")}</span>
                      <select
                        value={str(phase, "speed")}
                        onChange={(e) => patchPhase(phase, { speed: e.target.value })}
                      >
                        <option value="">—</option>
                        {SPEEDS.map((s) => (
                          <option key={s} value={s}>
                            {s}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      <span>{t("persona.motionBias.rhythm")}</span>
                      <select
                        value={str(phase, "rhythm")}
                        onChange={(e) => patchPhase(phase, { rhythm: e.target.value })}
                      >
                        <option value="">—</option>
                        {RHYTHMS.map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      <span>{t("persona.motionBias.intensity")}</span>
                      <input
                        type="number"
                        min={0}
                        max={100}
                        value={num(phase, "intensity")}
                        onChange={(e) =>
                          patchPhase(phase, { intensity: Number(e.target.value) })
                        }
                      />
                    </label>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="json-raw-block">
          <textarea
            className="mono json-raw-area"
            rows={12}
            value={rawDraft}
            onChange={(e) => {
              setRawDraft(e.target.value);
              setRawError(null);
            }}
            spellCheck={false}
          />
          {rawError && <p className="hint json-raw-error">{rawError}</p>}
          <button type="button" className="btn btn-sm btn-ghost" onClick={applyRaw}>
            {t("config.raw.apply")}
          </button>
        </div>
      )}
    </section>
  );
}
