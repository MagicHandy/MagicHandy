import { useId, useState } from "react";
import { api } from "../api/client";

export type HostPathKind = "executable" | "gguf" | "wav" | "npy" | "file" | "directory";

export function HostPathField({
  label, value, kind, disabled, placeholder, onChange,
}: {
  label: string;
  value: string;
  kind: HostPathKind;
  disabled?: boolean;
  placeholder?: string;
  onChange: (value: string) => void;
}) {
  const id = useId();
  const hintID = `${id}-hint`;
  const errorID = `${id}-error`;
  const [browsing, setBrowsing] = useState(false);
  const [error, setError] = useState("");

  async function browse() {
    setBrowsing(true);
    setError("");
    try {
      const result = await api.pickHostPath(kind, value);
      if (!result.canceled && result.path) onChange(result.path);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The path picker could not be opened.");
    } finally {
      setBrowsing(false);
    }
  }

  return (
    <div className="field host-path-field">
      <label className="label" htmlFor={id}>{label}</label>
      <div className="host-path-row">
        <input
          id={id}
          type="text"
          value={value}
          placeholder={placeholder}
          disabled={disabled || browsing}
          aria-invalid={error ? true : undefined}
          aria-describedby={`${hintID}${error ? ` ${errorID}` : ""}`}
          onChange={(event) => onChange(event.target.value)}
        />
        <button type="button" className="btn btn-secondary" disabled={disabled || browsing} onClick={() => void browse()} aria-label={`Browse for ${label}`}>
          {browsing ? "Opening..." : "Browse..."}
        </button>
      </div>
      <span id={hintID} className="hint-inline">Path on the computer running MagicHandy.</span>
      {error && <span id={errorID} className="form-status voice-worker-error" role="alert">{error}</span>}
    </div>
  );
}
