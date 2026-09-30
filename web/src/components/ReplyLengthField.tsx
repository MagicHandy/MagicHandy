import { t } from "../i18n";

const LENGTHS = ["short", "balanced", "detailed"] as const;

export function replyLengthLabel(length: string): string {
  if (length === "short") return t("Short (one or two sentences)");
  if (length === "detailed") return t("Detailed (up to five sentences)");
  return t("Balanced (the tested default)");
}

// The reply length select shared by Settings and the persona editor. A
// persona can also follow Settings, which the editor offers as an extra
// first option through `followLabel`.
export function ReplyLengthField({ value, options, locked, onChange, followLabel, label }: {
  value: string;
  options: string[];
  locked: boolean;
  onChange: (value: string) => void;
  followLabel?: string;
  label?: string;
}) {
  const lengths = options.length ? options : [...LENGTHS];
  return (
    <>
      <label className="field">
        <span className="label">{label ?? t("Reply length")}</span>
        <select value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          {followLabel !== undefined && <option value="">{followLabel}</option>}
          {lengths.map((length) => <option key={length} value={length}>{replyLengthLabel(length)}</option>)}
        </select>
      </label>
      <p className="hint">{t("Short suits voice output and quick back-and-forth. Detailed allows longer replies and raises the output limit to fit them. Reply length changes wording only; motion is unaffected.")}</p>
    </>
  );
}
