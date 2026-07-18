// Prompt sets: create/duplicate/edit/delete. Built-in sets are protected
// templates (read-only; duplicate to edit). The motion JSON contract is
// appended by the backend and cannot be edited out.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { PromptSet } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function PromptSetEditor({ locked = false }: { locked?: boolean }) {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [sets, setSets] = useState<PromptSet[] | null>(null);
  const [selId, setSelId] = useState<string>("");
  const [name, setName] = useState("");
  const [system, setSystem] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [busyAction, setBusyAction] = useState("");
  const mounted = useRef(false);
  const loadGeneration = useRef(0);
  const busyRef = useRef(false);

  async function reload(keep?: string) {
    if (!mounted.current) return false;
    const generation = ++loadGeneration.current;
    setLoading(true);
    setLoadError("");
    try {
      const res = await api.getPromptSets();
      if (!mounted.current || generation !== loadGeneration.current) return false;
      const list = Array.isArray(res.sets) ? res.sets : [];
      setSets(list);
      const pick = list.find((s) => s.id === (keep ?? selId)) ?? list[0];
      if (pick) {
        setSelId(pick.id);
        setName(pick.name);
        setSystem(pick.system);
      } else {
        setSelId("");
        setName("");
        setSystem("");
      }
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

  const selected = sets?.find((s) => s.id === selId);
  const builtin = selected?.builtin === true;
  const busy = Boolean(busyAction);
  const interactionLocked = locked || busy || loading;

  function select(id: string) {
    const s = sets?.find((x) => x.id === id);
    setSelId(id);
    if (s) {
      setName(s.name);
      setSystem(s.system);
    }
  }

  async function mutate(action: string, fn: () => Promise<void>) {
    if (busyRef.current || locked || loading) return;
    busyRef.current = true;
    setBusyAction(action);
    try {
      await fn();
    } catch (e) {
      if (mounted.current) show(msg(e), "error");
    } finally {
      busyRef.current = false;
      if (mounted.current) setBusyAction("");
    }
  }

  async function save() {
    if (builtin || !selected) return;
    await mutate("Saving prompt set", async () => {
      await api.updatePromptSet(selected.id, name.trim(), system.trim());
      show("Prompt set saved.");
      await reload(selected.id);
    });
  }
  async function duplicate() {
    if (!selected) return;
    await mutate("Duplicating prompt set", async () => {
      const created = await api.createPromptSet(`${name || "Prompt set"} copy`, system || "You are a helpful assistant.");
      show("Duplicated.");
      await reload(created.set?.id);
    });
  }
  async function remove() {
    if (builtin || !selected) return;
    await mutate("Deleting prompt set", async () => {
      await api.deletePromptSet(selected.id);
      show("Deleted.");
      await reload("");
    });
  }

  if (!sets) return (
    <div className="group">
      <h3 className="group-title">Prompt set editor</h3>
      {loadError ? (
        <div className="empty-state compact-empty" role="alert">
          <strong>Prompt sets unavailable</strong>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void reload()}>Retry</button>
        </div>
      ) : <p className="form-status" role="status">{loading ? "Loading prompt sets..." : "Prompt sets unavailable."}</p>}
    </div>
  );

  return (
    <div className="group" aria-busy={busy || loading || undefined}>
      <h3 className="group-title">Prompt set editor</h3>
      {loadError && (
        <div className="empty-state compact-empty" role="alert">
          <strong>Prompt set refresh failed</strong>
          <p>{loadError}</p>
          <button type="button" className="btn btn-secondary" disabled={busy || loading} onClick={() => void reload()}>Retry</button>
        </div>
      )}
      {sets.length === 0 && <p className="form-status" role="status">No prompt sets available.</p>}
      <label className="field">
        <span className="label">Edit set</span>
        <select value={selId} disabled={!backendOnline || busy || loading || sets.length === 0} onChange={(e) => select(e.target.value)}>
          {sets.map((s) => (
            <option key={s.id} value={s.id}>{s.name}{s.builtin ? " (built-in)" : ""}</option>
          ))}
        </select>
      </label>
      <label className="field">
        <span className="label">
          Name {builtin && <span className="badge">Built-in — read-only</span>}
        </span>
        <input type="text" maxLength={80} value={name} readOnly={builtin || interactionLocked || !selected} onChange={(e) => setName(e.target.value)} />
      </label>
      <label className="field">
        <span className="label">
          Behavior instructions <span className="hint-inline">the motion JSON contract is enforced by code</span>
        </span>
        <textarea rows={6} maxLength={16384} value={system} readOnly={builtin || interactionLocked || !selected} onChange={(e) => setSystem(e.target.value)} />
      </label>
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={interactionLocked || !selected} onClick={() => void duplicate()}>Duplicate as new</button>
        <button type="button" className="btn btn-primary" disabled={interactionLocked || builtin || !selected} onClick={() => void save()}>Save set</button>
        <button type="button" className="btn btn-danger-outline" disabled={interactionLocked || builtin || !selected} onClick={() => void remove()}>Delete set</button>
      </div>
      {busyAction && <p className="form-status" role="status">{busyAction}...</p>}
      {locked && <p className="form-status">{backendOnline ? "Read-only client." : "Core offline."}</p>}
    </div>
  );
}
