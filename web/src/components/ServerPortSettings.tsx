import { useId, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";

/** Settings > Access: the port the local server listens on. It applies at
 * the next start, so it keeps an explicit Save. */
export function ServerPortSettings() {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const id = useId();
  const hintId = useId();
  const saved = state?.settings?.server?.port;
  if (state?.capabilities?.configure_host === false || saved === undefined) return null;
  const locked = !backendOnline || readOnly;
  const value = draft ?? String(saved);
  const port = Number(value);
  const valid = Number.isInteger(port) && port >= 1 && port <= 65535;

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!valid || port === saved) return;
    setSaving(true);
    try {
      await api.saveServerPort(port);
      setDraft(null);
      show(t("Server port saved. It applies the next time MagicHandy starts."));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Request failed"), "error");
    } finally {
      setSaving(false);
      void refresh();
    }
  }

  return (
    <section className="group">
      <h3 className="group-title">{t("Local server")}</h3>
      <form className="form-rows" onSubmit={(event) => void save(event)}>
        <div className="form-row">
          <span className="form-row-label">
            <label htmlFor={id}><strong>{t("Server port")}</strong></label>
            <small id={hintId}>{t("The port MagicHandy listens on. A change applies the next time it starts.")}</small>
          </span>
          <div className="form-input-action">
            <input id={id} type="number" inputMode="numeric" min={1} max={65535} value={value} aria-describedby={hintId}
              disabled={locked || saving} onChange={(event) => setDraft(event.target.value)} />
            <button type="submit" className="btn btn-secondary" disabled={locked || saving || !valid || port === saved}>{saving ? t("Saving") : t("Save")}</button>
          </div>
        </div>
      </form>
    </section>
  );
}
