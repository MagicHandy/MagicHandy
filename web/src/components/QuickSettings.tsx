// Immediate-apply quick controls: speed/stroke/reverse/style. No save step —
// each change patches the engine live (docs/ui-design.md, Quick Controls).
// Disabled for read-only or backend-offline clients with a visible reason.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MotionSettings } from "../api/types";
import { RangeSlider } from "./RangeSlider";
import { useAppState, useToast } from "../state/app-state";

const STYLES = ["gentle", "balanced", "intense"] as const;
type QuickPatch = Parameters<typeof api.applyQuick>[0];
type QuickKey = keyof QuickPatch;

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
      if (mounted.current) show(e instanceof Error ? e.message : "Quick setting failed", "error");
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
          minGap={0}
          disabled={locked}
          onChange={({ min, max }, changed) => {
            setVals((s) => (s ? { ...s, speed_min_percent: min, speed_max_percent: max } : s));
            push(changed === "min" ? { speed_min_percent: min } : { speed_max_percent: max });
          }}
        />
      )}
      {showLimits && (
        <RangeSlider
          label="Stroke"
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
