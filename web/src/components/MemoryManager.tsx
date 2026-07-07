// Long-term memory: transparent and user-managed. Global switch, per-item
// enable/remove, add, and a double-confirm clear. Chat works with memory off.
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { MemoryState } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function MemoryManager() {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [mem, setMem] = useState<MemoryState | null>(null);
  const [draft, setDraft] = useState("");
  const [confirmClear, setConfirmClear] = useState(false);

  async function reload() {
    try {
      setMem(await api.getMemory());
    } catch (e) {
      show(msg(e), "error");
    }
  }
  useEffect(() => {
    void reload();
  }, []);

  async function run(fn: () => Promise<unknown>) {
    try {
      await fn();
      await reload();
    } catch (e) {
      show(msg(e), "error");
    }
  }

  if (!mem) return <p className="form-status">Loading memory…</p>;

  return (
    <div className="group">
      <h3 className="group-title">Long-term memory</h3>
      <label className="toggle-line" style={{ marginBottom: 12 }}>
        <span className="toggle">
          <input
            type="checkbox"
            role="switch"
            checked={mem.enabled}
            disabled={!backendOnline}
            onChange={(e) => void run(() => api.setMemoryEnabled(e.target.checked))}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>Include saved memories in chat <span className="hint-inline">applies immediately</span></span>
      </label>

      <ul style={{ listStyle: "none", margin: "0 0 12px", padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
        {mem.memories.length === 0 && <li className="form-status">No memories saved.</li>}
        {mem.memories.map((m) => (
          <li key={m.id} className="group" style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 10px" }}>
            <label className="toggle">
              <input
                type="checkbox"
                role="switch"
                checked={m.enabled}
                disabled={!backendOnline}
                aria-label="Enable memory"
                onChange={(e) => void run(() => api.setMemoryItemEnabled(m.id, e.target.checked))}
              />
              <span className="track" aria-hidden="true" />
            </label>
            <span style={{ flex: 1, minWidth: 0, overflowWrap: "anywhere", color: "var(--text)" }}>{m.text}</span>
            <button type="button" className="btn btn-secondary" disabled={!backendOnline} onClick={() => void run(() => api.removeMemory(m.id))}>
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
          disabled={!backendOnline}
          placeholder="A short fact the assistant should remember"
          onChange={(e) => setDraft(e.target.value)}
        />
      </label>
      <div className="row-actions">
        <button
          type="button"
          className="btn btn-primary"
          disabled={!backendOnline || !draft.trim()}
          onClick={() => void run(async () => { await api.addMemory(draft.trim()); setDraft(""); })}
        >
          Add memory
        </button>
        <button
          type="button"
          className="btn btn-danger-outline"
          disabled={!backendOnline || mem.memories.length === 0}
          onClick={() => {
            if (!confirmClear) {
              setConfirmClear(true);
              return;
            }
            setConfirmClear(false);
            void run(() => api.clearMemory());
          }}
        >
          {confirmClear ? "Confirm clear all" : "Clear all"}
        </button>
      </div>
    </div>
  );
}
