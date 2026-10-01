// Freestyle preferences: a feel preset, five named-stop tendencies, an accent
// and a visible session shape. Every change applies immediately; the backend
// eases the running stream into it after the motion already queued.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { FreestyleSettings, FreestyleStatus } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { SegmentedChoice, SetpointSlider } from "./SetpointControls";

type ScaleKey = "pace_percent" | "length_percent" | "focus_percent" | "roaming_percent" | "variety_percent";

const feelOptions = [
  ["gentle", "Gentle"],
  ["balanced", "Balanced"],
  ["intense", "Intense"],
  ["custom", "Custom"],
] as const satisfies ReadonlyArray<readonly [string, MessageKey]>;

const accentOptions = [
  ["even", "Even"],
  ["tip", "Quicker up"],
  ["base", "Quicker down"],
] as const satisfies ReadonlyArray<readonly [string, MessageKey]>;

const shapeOptions = [
  ["steady", "Steady"],
  ["build", "Slow build"],
  ["waves", "Waves"],
  ["edge", "Edge"],
  ["cooldown", "Cooldown"],
] as const satisfies ReadonlyArray<readonly [string, MessageKey]>;

const scales: ReadonlyArray<{
  key: ScaleKey;
  label: MessageKey;
  stops: ReadonlyArray<readonly [number, MessageKey]>;
}> = [
  { key: "pace_percent", label: "Pace", stops: [[0, "Slowest"], [25, "Slow"], [50, "Medium"], [75, "Brisk"], [100, "Fastest"]] },
  { key: "length_percent", label: "Stroke length", stops: [[0, "Short"], [25, "Partial"], [50, "Half"], [75, "Long"], [100, "Full stroke"]] },
  { key: "focus_percent", label: "Focus", stops: [[0, "Base"], [25, "Lower"], [50, "Middle"], [75, "Upper"], [100, "Tip"]] },
  { key: "roaming_percent", label: "Roaming", stops: [[0, "Stays put"], [25, "Drifts"], [50, "Wanders"], [75, "Roams"], [100, "Everywhere"]] },
  { key: "variety_percent", label: "Variety", stops: [[0, "Even"], [25, "Subtle"], [50, "Natural"], [75, "Lively"], [100, "Wild"]] },
];

const phaseCopy: Partial<Record<string, MessageKey>> = {
  building: "Building",
  holding: "Holding the pace",
  rising: "Rising",
  falling: "Falling",
  peak: "At the peak",
  backing_off: "Backing off",
  teasing: "Teasing at the tip",
  easing: "Winding down",
  finished: "Coming to rest",
};

const minimumShapeMinutes = 1;
const maximumShapeMinutes = 240;

// nearestStop shows a value saved by another client on the closest named stop.
function nearestStop(value: number, stops: ReadonlyArray<readonly [number, MessageKey]>): string {
  let best = stops[0][0];
  for (const [stop] of stops) {
    if (Math.abs(stop - value) < Math.abs(best - value)) best = stop;
  }
  return String(best);
}

export function freestyleShapeStatus(status: FreestyleStatus | undefined): string {
  if (!status || status.shape === "steady") return "";
  const shape = shapeOptions.find(([value]) => value === status.shape)?.[1];
  const phase = status.shape_phase ? phaseCopy[status.shape_phase] : undefined;
  const parts = [shape ? translateKnown(shape) : status.shape];
  if (phase) parts.push(translateKnown(phase));
  if (status.shape === "build" || status.shape === "cooldown") {
    parts.push(`${status.shape_progress_percent ?? 0}%`);
  }
  return parts.join(" · ");
}

export function FreestylePreferences({
  value,
  disabled,
  onSaved,
  onError,
}: {
  value: FreestyleSettings;
  disabled: boolean;
  onSaved: () => void;
  onError: (error: unknown) => void;
}) {
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const draftRef = useRef(value);
  const editingRef = useRef(false);
  const savingRef = useRef(false);
  const queuedSaveRef = useRef<Partial<FreestyleSettings> | null>(null);

  useEffect(() => {
    if (savingRef.current || editingRef.current) return;
    draftRef.current = value;
    setDraft(value);
  }, [value]);

  async function save(patch: Partial<FreestyleSettings>) {
    if (disabled) return;
    editingRef.current = false;
    const next = { ...draftRef.current, ...patch };
    draftRef.current = next;
    setDraft(next);
    if (savingRef.current) {
      queuedSaveRef.current = { ...queuedSaveRef.current, ...patch };
      return;
    }
    savingRef.current = true;
    setSaving(true);
    try {
      let requested: FreestyleSettings | null = next;
      while (requested) {
        queuedSaveRef.current = null;
        const response = await api.saveFreestylePreferences(requested);
        const queued = queuedSaveRef.current as Partial<FreestyleSettings> | null;
        if (!queued) {
          draftRef.current = response.freestyle;
          setDraft(response.freestyle);
        }
        onSaved();
        // Only the fields edited while waiting belong in the next request.
        // A named feel's other values come from the backend acknowledgement.
        requested = queued ? { ...response.freestyle, ...queued } : null;
        if (requested) {
          draftRef.current = requested;
          setDraft(requested);
        }
      }
    } catch (error) {
      queuedSaveRef.current = null;
      draftRef.current = value;
      setDraft(value);
      onError(error);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }

  function saveMinutes(raw: string) {
    const parsed = Number.parseInt(raw, 10);
    const minutes = Number.isFinite(parsed)
      ? Math.min(maximumShapeMinutes, Math.max(minimumShapeMinutes, parsed))
      : 15;
    void save({ shape_minutes: minutes });
  }

  const timed = draft.shape === "build" || draft.shape === "cooldown";
  return (
    <fieldset className="freestyle-preferences" disabled={disabled} aria-busy={saving || undefined}>
      <legend className="visually-hidden">{t("Freestyle feel")}</legend>
      <SegmentedChoice
        className="freestyle-feel"
        label={t("Feel")}
        value={draft.feel}
        options={feelOptions.map(([option, label]) => ({ value: option, label: translateKnown(label) }))}
        disabled={disabled}
        onChange={(feel) => void save({ feel })}
      />
      <div className="freestyle-scales">
        {scales.map(({ key, label, stops }) => (
          <SetpointSlider
            key={key}
            className="freestyle-scale"
            label={translateKnown(label)}
            value={nearestStop(draft[key], stops)}
            options={stops.map(([stop, stopLabel]) => ({ value: String(stop), label: translateKnown(stopLabel) }))}
            disabled={disabled}
            onChange={(stop) => void save({ feel: "custom", [key]: Number(stop) })}
          />
        ))}
      </div>
      <SegmentedChoice
        className="freestyle-accent"
        label={t("Accent")}
        value={draft.accent}
        options={accentOptions.map(([option, label]) => ({ value: option, label: translateKnown(label) }))}
        disabled={disabled}
        onChange={(accent) => void save({ feel: "custom", accent })}
      />
      <SegmentedChoice
        className="freestyle-shape"
        label={t("Session shape")}
        value={draft.shape}
        options={shapeOptions.map(([option, label]) => ({ value: option, label: translateKnown(label) }))}
        disabled={disabled}
        onChange={(shape) => void save({ shape })}
      />
      {timed && (
        <div className="freestyle-window">
          <span>{draft.shape === "build" ? t("Build over") : t("Wind down over")}</span>
          <input
            type="number"
            min={minimumShapeMinutes}
            max={maximumShapeMinutes}
            value={draft.shape_minutes}
            aria-label={t("Shape minutes")}
            onChange={(event) => {
              editingRef.current = true;
              const next = { ...draftRef.current, shape_minutes: Number(event.target.value) };
              draftRef.current = next;
              setDraft(next);
            }}
            onBlur={(event) => saveMinutes(event.target.value)}
          />
          <span>{t("minutes")}</span>
        </div>
      )}
      <small className="freestyle-hint">{t("Everything stays inside your speed and stroke limits.")}</small>
    </fieldset>
  );
}
