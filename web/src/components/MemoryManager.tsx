// Long-term memory: transparent and user-managed. Global switch, per-item
// enable/remove, add, and a double-confirm clear. Chat works with memory off.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MemoryState } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function MemoryManager({ locked = false }: { locked?: boolean }) {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [mem, setMem] = useState<MemoryState | null>(null);
  const [draft, setDraft] = useState("");
  const [confirmClear, setConfirmClear] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [busyAction, setBusyAction] = useState("");
  const mounted = useRef(false);
  const loadGeneration = useRef(0);
  const busyRef = useRef(false);

  async function reload() {
    if (!mounted.current) return false;
    const generation = ++loadGeneration.current;
    setLoading(true);
    setLoadError("");
    try {
      const next = await api.getMemory();
      if (!mounted.current || generation !== loadGeneration.current) return false;
      setMem(next);
      return true;
    } catch (e) {
      if (mounted.current && generation === loadGeneration.current) setLoadError(msg(e));
      return false;
    } finally {
      if (mounted.current && generation === loadGeneration.current) setLoading(false);
    }
  }
  useEffect(() => {
    mounted.current = true;
    void reload();
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
    };
  }, []);

  async function run(action: string, fn: () => Promise<unknown>) {
    if (busyRef.current || locked || loading) return;
    busyRef.current = true;
    setBusyAction(action);
    try {
      await fn();
      if (mounted.current) await reload();
    } catch (e) {
      if (mounted.current) show(msg(e), "error");
    } finally {
      busyRef.current = false;
      if (mounted.current) setBusyAction("");
    }
  }

  if (!mem) return (
    <div className="group">
      <h3 className="group-title">Long-term memory</h3>
      {loadError ? (
        <div className="empty-state compact-empty" role="alert">
          <strong>Memory unavailable</strong>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void reload()}>Retry</button>
        </div>
      ) : <p className="form-status" role="status">{loading ? "Loading memory..." : "Memory unavailable."}</p>}
    </div>
  );
  const memories = Array.isArray(mem.memories) ? mem.memories : [];
  const busy = Boolean(busyAction);
  const interactionLocked = locked || busy || loading;

  return (
    <div className="group" aria-busy={busy || loading || undefined}>
      <h3 className="group-title">Long-term memory</h3>
      {loadError && (
        <div className="empty-state compact-empty" role="alert">
          <strong>Memory refresh failed</strong>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" disabled={busy || loading} onClick={() => void reload()}>Retry</button>
        </div>
      )}
      <label className="toggle-line hint-block">
        <span className="toggle">
          <input
            type="checkbox"
            role="switch"
            checked={mem.enabled}
            disabled={interactionLocked}
            onChange={(e) => void run("Updating memory", () => api.setMemoryEnabled(e.target.checked))}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>Include saved memories in chat <span className="hint-inline">applies immediately</span></span>
      </label>

      <ul className="memory-list">
        {memories.length === 0 && <li className="form-status">No memories saved.</li>}
        {memories.map((m) => (
          <li key={m.id} className="group memory-item">
            <label className="toggle">
              <input
                type="checkbox"
                role="switch"
                checked={m.enabled}
                disabled={interactionLocked}
                aria-label="Enable memory"
                onChange={(e) => void run("Updating memory", () => api.setMemoryItemEnabled(m.id, e.target.checked))}
              />
              <span className="track" aria-hidden="true" />
            </label>
            <span className="memory-text">{m.text}</span>
            <button type="button" className="btn btn-secondary" disabled={interactionLocked} onClick={() => void run("Removing memory", () => api.removeMemory(m.id))}>
              Remove
            </button>
          </li>
        ))}
      </ul>

      <label className="field">
        <span className="label">New memory</span>
        <textarea
          rows={2}
          maxLength={2000}
          value={draft}
          disabled={interactionLocked}
          placeholder="A short fact the assistant should remember"
          onChange={(e) => setDraft(e.target.value)}
        />
      </label>
      <div className="row-actions">
        <button
          type="button"
          className="btn btn-primary"
          disabled={interactionLocked || !draft.trim()}
          onClick={() => void run("Adding memory", async () => { await api.addMemory(draft.trim()); if (mounted.current) setDraft(""); })}
        >
          Add memory
        </button>
        <button
          type="button"
          className="btn btn-danger-outline"
          disabled={interactionLocked || memories.length === 0}
          onClick={() => {
            if (!confirmClear) {
              setConfirmClear(true);
              return;
            }
            setConfirmClear(false);
            void run("Clearing memory", () => api.clearMemory());
          }}
        >
          {confirmClear ? "Confirm clear all" : "Clear all"}
        </button>
      </div>
      {busyAction && <p className="form-status" role="status">{busyAction}...</p>}
      {locked && <p className="form-status">{backendOnline ? "Read-only client." : "Core offline."}</p>}
    </div>
  );
}
