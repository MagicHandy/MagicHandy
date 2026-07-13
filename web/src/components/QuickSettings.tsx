// Immediate-apply quick controls: speed/stroke/reverse/style. No save step —
// each change patches the engine live (docs/ui-design.md, Quick Controls).
// Disabled for read-only or backend-offline clients with a visible reason.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MotionSettings } from "../api/types";
import { RangeSlider } from "./RangeSlider";
import { useAppState, useToast } from "../state/app-state";

const STYLES = ["gentle", "balanced", "intense"] as const;

interface QuickSettingsProps {
  section?: "all" | "limits" | "behavior";
}

export function QuickSettings({ section = "all" }: QuickSettingsProps) {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const motion = state?.settings?.motion;
  const locked = !backendOnline || readOnly;
  const [vals, setVals] = useState<MotionSettings | null>(null);
  const timer = useRef<number | undefined>(undefined);
  const pending = useRef<Record<string, unknown>>({});

  useEffect(() => {
    if (motion) setVals({ ...motion });
  }, [motion]);

  // Debounced, but patches accumulate so moving one range thumb never drops a
  // sibling value queued in the same window.
  function push(patch: Record<string, unknown>) {
    pending.current = { ...pending.current, ...patch };
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      const body = pending.current;
      pending.current = {};
      try {
        await api.applyQuick(body);
      } catch (e) {
        show(e instanceof Error ? e.message : "Quick setting failed", "error");
      } finally {
        refresh();
      }
    }, 180);
  }

  if (!vals) return <p className="form-status">Loading…</p>;

  const showLimits = section !== "behavior";
  const showBehavior = section !== "limits";

  return (
    <fieldset className={`quick-fields quick-fields-${section}`} disabled={locked}>
      <legend className="visually-hidden">{section === "limits" ? "Speed and stroke limits" : section === "behavior" ? "Direction and motion style" : "Speed, stroke, direction, and style"}</legend>
      {showLimits && (
        <RangeSlider
          label="Speed"
          floor={1}
          minValue={vals.speed_min_percent}
          maxValue={vals.speed_max_percent}
          disabled={locked}
          onChange={({ min, max }) => {
            setVals((s) => (s ? { ...s, speed_min_percent: min, speed_max_percent: max } : s));
            push({ speed_min_percent: min, speed_max_percent: max });
          }}
        />
      )}
      {showLimits && (
        <RangeSlider
          label="Stroke"
          floor={0}
          minValue={vals.stroke_min_percent}
          maxValue={vals.stroke_max_percent}
          disabled={locked}
          onChange={({ min, max }) => {
            setVals((s) => (s ? { ...s, stroke_min_percent: min, stroke_max_percent: max } : s));
            push({ stroke_min_percent: min, stroke_max_percent: max });
          }}
        />
      )}
      {showBehavior && <label className="toggle-line">
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
      </label>}
      {showBehavior && <label className="field">
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
      </label>}
      {locked && (
        <p className="form-status">{readOnly ? "Read-only client — cannot change motion." : "Core offline."}</p>
      )}
    </fieldset>
  );
}
