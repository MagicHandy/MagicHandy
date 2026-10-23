import { t, translateKnown } from "../i18n";
// Immediate-apply quick controls: speed/stroke/reverse/style and the Handy's
// calibration. No save step — each change patches the engine live
// (docs/ui-design.md, Quick Controls). Disabled for read-only or
// backend-offline clients with a visible reason.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { isHandy, playbackDevices } from "../api/devices";
import type { HandyModel, MotionSettings } from "../api/types";
import { RangeSlider } from "./RangeSlider";
import { SegmentedChoice } from "./SetpointControls";
import { useAppState, useToast } from "../state/app-state";
import { useApplicationAudience } from "../state/application-audience";

const STYLES = ["gentle", "balanced", "intense"] as const;
export const DEFAULT_HANDY_MODELS: HandyModel[] = ["handy_original", "handy_2_standard", "handy_2_pro"];
type QuickPatch = Parameters<typeof api.applyQuick>[0];
type QuickKey = keyof QuickPatch;

interface QuickSettingsProps {
  /** limits: the device panel; calibration: the Handy's page; style: Chat. */
  section?: "all" | "limits" | "style" | "calibration";
}

export function QuickSettings({ section = "all" }: QuickSettingsProps) {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const audience = useApplicationAudience();
  const motion = state?.settings?.motion;
  const handyModels = state?.settings?.options?.handy_models ?? DEFAULT_HANDY_MODELS;
  const locked = !backendOnline || readOnly;
  const [vals, setVals] = useState<MotionSettings | null>(null);
  const timer = useRef<number | undefined>(undefined);
  const pending = useRef<QuickPatch>({});
  const motionRef = useRef(motion);
  const revision = useRef(0);
  const dirtyRevisions = useRef(new Map<QuickKey, number>());
  const desiredValues = useRef(new Map<QuickKey, MotionSettings[QuickKey]>());
  const sending = useRef(false);
  const mounted = useRef(true);

  useEffect(() => {
    motionRef.current = motion;
    if (motion) {
      for (const key of dirtyRevisions.current.keys()) {
        if (Object.is(motion[key], desiredValues.current.get(key))) {
          dirtyRevisions.current.delete(key);
          desiredValues.current.delete(key);
        }
      }
      setVals((current) => reconcileMotion(current, motion, dirtyRevisions.current));
    }
  }, [motion]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      window.clearTimeout(timer.current);
      if (quickKeys(pending.current).length) void flush();
    };
  }, []);

  // Combine rapid edits without resending untouched bounds from a stale poll.
  function push(patch: QuickPatch) {
    if (locked || (patch.handy_model !== undefined && state?.capabilities?.configure_host === false)) return;
    revision.current += 1;
    for (const key of quickKeys(patch)) {
      dirtyRevisions.current.set(key, revision.current);
      desiredValues.current.set(key, patch[key] as MotionSettings[QuickKey]);
    }
    pending.current = { ...pending.current, ...patch };
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => void flush(), 180);
  }

  async function flush() {
    if (!audience.active) { pending.current = {}; return; }
    if (sending.current) return;
    const body = pending.current;
    const keys = quickKeys(body);
    if (!keys.length) return;
    pending.current = {};
    const sentRevisions = new Map(keys.map((key) => [key, dirtyRevisions.current.get(key)]));
    sending.current = true;
    let succeeded = false;
    try {
      await api.applyQuick(body);
      succeeded = true;
    } catch (e) {
      if (mounted.current) show(e instanceof Error ? translateKnown(e.message) : t("Quick setting failed."), "error");
    } finally {
      if (!succeeded) {
        for (const [key, sentRevision] of sentRevisions) {
          if (dirtyRevisions.current.get(key) === sentRevision) {
            dirtyRevisions.current.delete(key);
            desiredValues.current.delete(key);
          }
        }
      }
      sending.current = false;
      if (mounted.current) {
        const currentMotion = motionRef.current;
        if (!succeeded && currentMotion) {
          setVals((current) => reconcileMotion(current, currentMotion, dirtyRevisions.current));
        }
        refresh();
      }
      if (quickKeys(pending.current).length) {
        window.clearTimeout(timer.current);
        if (mounted.current) timer.current = window.setTimeout(() => void flush(), 0);
        else void flush();
      }
    }
  }

  if (!vals) return <p className="form-status">{t("Loading…")}</p>;

  // Without a Handy in playback, only Speed still matters: it sets the pace
  // the other devices follow. The stroke window is for every Handy that
  // plays. Calibration lives on the Handy's own page, so it shows whether or
  // not it plays.
  const handyShown = playbackDevices(state?.settings?.device).some(isHandy);
  const calibration = section === "calibration";
  const showLimits = section === "all" || section === "limits";
  const showDirection = calibration || (handyShown && section === "all");
  const showStyle = section === "all" || section === "style";
  const showHandyModel = calibration;
  const legend = section === "limits"
    ? t("Speed and stroke ranges")
    : section === "style"
      ? t("Motion style")
      : calibration
        ? t("Calibration")
        : t("Speed, stroke, direction, and style");

  return (
    <fieldset className={`quick-fields quick-fields-${section}${calibration ? " form-rows device-rows" : ""}`} disabled={locked}>
      <legend className="visually-hidden">{legend}</legend>
      {showHandyModel && <div className="form-row device-option-row quick-handy-model-row">
        <span className="form-row-label"><strong>{t("Model")}</strong><small>{t("Sets the travel for millimetres and speed limits.")}</small></span>
        <div className="device-option-control">
          <SegmentedChoice
            className="quick-handy-model device-segmented"
            label={t("Handy model")}
            disabled={state?.capabilities?.configure_host === false}
            value={vals.handy_model}
            options={handyModels.map((model) => ({ value: model, label: handyModelLabel(model) }))}
            onChange={(handy_model) => {
              setVals((current) => (current ? { ...current, handy_model } : current));
              push({ handy_model });
            }}
          />
          <small className="quick-handy-spec">{handyModelDetail(vals.handy_model)}</small>
        </div>
      </div>}
      {showLimits && (
        <RangeSlider
          label={t("Speed")}
          floor={1}
          minValue={vals.speed_min_percent}
          maxValue={vals.speed_max_percent}
          minGap={0}
          disabled={locked}
          onChange={({ min, max }, changed) => {
            setVals((s) => (s ? { ...s, speed_min_percent: min, speed_max_percent: max } : s));
            push(changed === "min" ? { speed_min_percent: min } : { speed_max_percent: max });
          }}
        />
      )}
      {showLimits && handyShown && (
        <RangeSlider
          label={t("Stroke")}
          floor={0}
          minValue={vals.stroke_min_percent}
          maxValue={vals.stroke_max_percent}
          minGap={1}
          disabled={locked}
          onChange={({ min, max }, changed) => {
            setVals((s) => (s ? { ...s, stroke_min_percent: min, stroke_max_percent: max } : s));
            push(changed === "min" ? { stroke_min_percent: min } : { stroke_max_percent: max });
          }}
        />
      )}
      {showDirection && <label className={calibration ? "form-row toggle-line device-switch-row quick-direction-toggle" : "toggle-line quick-direction-toggle"}>
        {calibration && <span className="form-row-label"><strong>{t("Reverse direction")}</strong><small>{t("For a Handy mounted the other way up.")}</small></span>}
        <span className="toggle">
          <input
            type="checkbox"
            role="switch"
            aria-label={calibration ? t("Reverse direction") : undefined}
            checked={vals.reverse_direction}
            onChange={(e) => {
              const v = e.target.checked;
              setVals((s) => (s ? { ...s, reverse_direction: v } : s));
              push({ reverse_direction: v });
            }}
          />
          <span className="track" aria-hidden="true" />
        </span>
        {!calibration && <span>{t("Reverse direction")}</span>}
      </label>}
      {showStyle && <SegmentedChoice
        className="quick-style-choice"
        label={t("Style")}
        hint={t("biases motion character")}
        value={vals.style}
        options={STYLES.map((style) => ({
          value: style,
          label: translateKnown(style[0].toUpperCase() + style.slice(1)),
        }))}
        onChange={(style) => {
          setVals((current) => (current ? { ...current, style } : current));
          push({ style });
        }}
      />}
      {locked && (
        <p className="form-status">{readOnly ? t("Read-only client — cannot change motion.") : t("Core offline.")}</p>
      )}
    </fieldset>
  );
}

export function handyModelLabel(model: string): string {
  switch (model) {
    case "handy_2_standard": return "2 Standard";
    case "handy_2_pro": return "2 Pro";
    default: return "Original";
  }
}

export function handyModelDetail(model: string): string {
  switch (model) {
    case "handy_2_standard": return "125 mm · 32–400 mm/s";
    case "handy_2_pro": return "125 mm · 32–450 mm/s";
    default: return "110 mm · 32–400 mm/s";
  }
}

function quickKeys(patch: QuickPatch): QuickKey[] {
  return Object.keys(patch) as QuickKey[];
}

function reconcileMotion(
  current: MotionSettings | null,
  server: MotionSettings,
  dirty: ReadonlyMap<QuickKey, number>,
): MotionSettings {
  if (!current || dirty.size === 0) return { ...server };
  const next = { ...server };
  for (const key of dirty.keys()) Object.assign(next, { [key]: current[key] });
  return next;
}
