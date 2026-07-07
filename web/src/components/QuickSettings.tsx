// Immediate-apply quick controls: speed/stroke/reverse/style. No save step —
// each change patches the engine live (docs/ui-design.md, Quick Controls).
// Disabled for read-only or backend-offline clients with a visible reason.
import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { api } from "../api/client";
import type { MotionSettings } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

const STYLES = ["gentle", "balanced", "intense"] as const;

export function QuickSettings() {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const motion = state?.settings?.motion;
  const locked = !backendOnline || readOnly;
  const [vals, setVals] = useState<MotionSettings | null>(null);
  const seeded = useRef(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (!seeded.current && motion) {
      setVals({ ...motion });
      seeded.current = true;
    }
  }, [motion]);

  function push(patch: Record<string, unknown>) {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      try {
        await api.applyQuick(patch);
      } catch (e) {
        show(e instanceof Error ? e.message : "Quick setting failed", "error");
      } finally {
        refresh();
      }
    }, 180);
  }

  if (!vals) return <p className="form-status">Loading…</p>;

  const range = (key: keyof MotionSettings, label: string, min: number) => (
    <label className="field">
      <span className="label">
        {label} <output>{vals[key] as number}%</output>
      </span>
      <input
        type="range"
        min={min}
        max={100}
        value={vals[key] as number}
        onChange={(e: ChangeEvent<HTMLInputElement>) => {
          const v = Number(e.target.value);
          setVals((s) => (s ? { ...s, [key]: v } : s));
          push({ [key]: v });
        }}
      />
    </label>
  );

  return (
    <fieldset className="quick-fields" disabled={locked}>
      <legend className="visually-hidden">Speed, stroke, direction, and style</legend>
      {range("speed_min_percent", "Speed min", 1)}
      {range("speed_max_percent", "Speed max", 1)}
      {range("stroke_min_percent", "Stroke min", 0)}
      {range("stroke_max_percent", "Stroke max", 0)}
      <label className="toggle-line">
        <span className="toggle">
          <input
            type="checkbox"
            role="switch"
            checked={vals.reverse_direction}
            onChange={(e) => {
              const v = e.target.checked;
              setVals((s) => (s ? { ...s, reverse_direction: v } : s));
              push({ reverse_direction: v });
            }}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>Reverse direction</span>
      </label>
      <label className="field">
        <span className="label">
          Style <span className="hint-inline">biases Freestyle pacing</span>
        </span>
        <select
          value={vals.style}
          onChange={(e) => {
            const v = e.target.value;
            setVals((s) => (s ? { ...s, style: v } : s));
            push({ style: v });
          }}
        >
          {STYLES.map((s) => (
            <option key={s} value={s}>{s[0].toUpperCase() + s.slice(1)}</option>
          ))}
        </select>
      </label>
      {locked && (
        <p className="form-status">{readOnly ? "Read-only client — cannot change motion." : "Core offline."}</p>
      )}
    </fieldset>
  );
}
