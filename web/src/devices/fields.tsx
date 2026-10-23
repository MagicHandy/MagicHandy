// Settings > Devices fields. Each one saves when it changes: a switch or a
// choice at once, a slider shortly after the drag settles, typed text when
// it is committed. The field shows its own edit until the backend snapshot
// catches up, and the snapshot again if the save fails.
import { useCallback, useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { api } from "../api/client";
import type { HandyConnectionPatch } from "../api/handy-types";
import type { DeviceOptionsPatch, HandyInstanceId } from "../api/types";
import { RangeInput } from "../components/RangeInput";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";

/** A slider's save waits for the drag to settle. */
export const SLIDER_SAVE_MS = 300;

/**
 * One saved value with its edit in flight. Saves are serialized: an edit
 * made while one is sending is sent when that one finishes.
 */
export function useLiveSetting<T>(server: T, save: (value: T) => Promise<void>, delay = 0): [T, (value: T) => void, () => void] {
  const [pending, setPending] = useState<{ value: T } | null>(null);
  const pendingRef = useRef(pending);
  pendingRef.current = pending;
  const queued = useRef<{ value: T } | null>(null);
  const sending = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const mounted = useRef(true);
  const saveRef = useRef(save);
  saveRef.current = save;

  const commit = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    const next = queued.current;
    if (sending.current || !next) return;
    queued.current = null;
    sending.current = true;
    void saveRef.current(next.value).catch(() => undefined).finally(() => {
      sending.current = false;
      if (queued.current) commit();
      else if (mounted.current && pendingRef.current === next) setPending(null);
    });
  }, []);

  const set = useCallback((value: T) => {
    const next = { value };
    queued.current = next;
    pendingRef.current = next;
    setPending(next);
    window.clearTimeout(timer.current);
    if (delay > 0) timer.current = window.setTimeout(commit, delay);
    else commit();
  }, [commit, delay]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      commit();
    };
  }, [commit]);

  return [pending ? pending.value : server, set, commit];
}

/** Saves one device option change, reports a failure, and refreshes. */
export function useSaveDeviceOptions(): (patch: DeviceOptionsPatch) => Promise<void> {
  return useSaveChange(useCallback((patch: DeviceOptionsPatch) => api.saveDeviceOptions(patch), []));
}

/** Saves one change to an additional Handy, the same way. */
export function useSaveHandy(id: HandyInstanceId): (patch: HandyConnectionPatch) => Promise<void> {
  return useSaveChange(useCallback((patch: HandyConnectionPatch) => api.updateHandy(id, patch), [id]));
}

function useSaveChange<P>(send: (patch: P) => Promise<unknown>): (patch: P) => Promise<void> {
  const { refresh } = useAppState();
  const { show } = useToast();
  return useCallback(async (patch: P) => {
    try {
      await send(patch);
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("The setting could not be saved."), "error");
      throw error;
    } finally {
      await refresh();
    }
  }, [send, refresh, show]);
}

/** A label and hint on the left, its control on the right. */
export function OptionRow({ label, hint, labelId, children, stack = false }: {
  label: string;
  hint?: ReactNode;
  labelId?: string;
  children: ReactNode;
  stack?: boolean;
}) {
  return (
    <div className={`form-row device-option-row${stack ? " form-row-stack" : ""}`}>
      <span className="form-row-label"><strong id={labelId}>{label}</strong>{hint && <small>{hint}</small>}</span>
      <div className="device-option-control">{children}</div>
    </div>
  );
}

/** An on/off option: label and hint left, switch right. */
export function SwitchRow({ label, hint, checked, disabled, onChange }: {
  label: string;
  hint?: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="form-row toggle-line device-switch-row">
      <span className="form-row-label"><strong>{label}</strong>{hint && <small>{hint}</small>}</span>
      <span className="toggle">
        <input type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={(event) => onChange(event.target.checked)} />
        <span className="track" aria-hidden="true" />
      </span>
    </label>
  );
}

/**
 * Typed text that saves on Enter or when focus leaves. Until then it is the
 * person's draft; afterwards it follows the saved value again.
 */
export function CommitInput({ value, onCommit, type = "text", disabled, className, ...rest }: {
  value: string;
  onCommit: (value: string) => void;
  type?: "text" | "number" | "url";
  disabled?: boolean;
  className?: string;
  id?: string;
  min?: number;
  max?: number;
  step?: number;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
  spellCheck?: boolean;
  inputMode?: "numeric" | "url" | "text";
}) {
  const [draft, setDraft] = useState<string | null>(null);
  function commit() {
    if (draft === null) return;
    const next = draft.trim();
    setDraft(null);
    if (next !== value) onCommit(next);
  }
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
    }
  }
  return (
    <input
      {...rest}
      className={className}
      type={type}
      value={draft ?? value}
      disabled={disabled}
      autoComplete="off"
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={onKeyDown}
    />
  );
}

/** A whole number field in a range, committed like CommitInput. */
export function NumberInput({ value, min, max, step = 1, onCommit, ...rest }: {
  value: number;
  min: number;
  max: number;
  step?: number;
  onCommit: (value: number) => void;
  disabled?: boolean;
  id?: string;
  className?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
}) {
  return (
    <CommitInput
      {...rest}
      type="number"
      inputMode="numeric"
      min={min}
      max={max}
      step={step}
      value={String(value)}
      onCommit={(text) => {
        const parsed = Math.round(Number(text));
        if (text === "" || !Number.isFinite(parsed)) return;
        onCommit(Math.max(min, Math.min(max, parsed)));
      }}
    />
  );
}

/** One value on the shared slider recipe, with its readout in the head. */
export function ValueSlider({ label, hideLabel = false, value, min, max, step = 1, valueText, ends, disabled, onChange }: {
  label: string;
  /** The row already names it: keep the name for assistive technology only. */
  hideLabel?: boolean;
  value: number;
  min: number;
  max: number;
  step?: number;
  valueText: string;
  /** Labels under each end of the track. */
  ends?: [string, string];
  disabled?: boolean;
  onChange: (value: number) => void;
}) {
  const id = useId();
  return (
    <div className="device-value-slider" data-disabled={disabled || undefined} data-centered={(hideLabel && ends) || undefined}>
      <div className="range-slider-head">
        <label className={hideLabel ? "label visually-hidden" : "label"} htmlFor={id}>{label}</label>
        <output htmlFor={id} className="range-slider-value">{valueText}</output>
      </div>
      <RangeInput id={id} min={min} max={max} step={step} value={value} disabled={disabled} aria-valuetext={valueText}
        onChange={(event) => onChange(Number(event.target.value))} />
      {ends && <div className="device-slider-ends" aria-hidden="true"><span>{ends[0]}</span><span>{ends[1]}</span></div>}
    </div>
  );
}
