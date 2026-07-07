// Prompt sets: create/duplicate/edit/delete. Built-in sets are protected
// templates (read-only; duplicate to edit). The motion JSON contract is
// appended by the backend and cannot be edited out.
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { PromptSet } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function PromptSetEditor({ locked = false }: { locked?: boolean }) {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [sets, setSets] = useState<PromptSet[]>([]);
  const [selId, setSelId] = useState<string>("");
  const [name, setName] = useState("");
  const [system, setSystem] = useState("");

  async function reload(keep?: string) {
    try {
      const res = await api.getPromptSets();
      const list = Array.isArray(res.sets) ? res.sets : [];
      setSets(list);
      const pick = list.find((s) => s.id === (keep ?? selId)) ?? list[0];
      if (pick) {
        setSelId(pick.id);
        setName(pick.name);
        setSystem(pick.system);
      }
    } catch (e) {
      show(msg(e), "error");
    }
  }
  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const selected = sets.find((s) => s.id === selId);
  const builtin = selected?.builtin === true;

  function select(id: string) {
    const s = sets.find((x) => x.id === id);
    setSelId(id);
    if (s) {
      setName(s.name);
      setSystem(s.system);
    }
  }

  async function save() {
    if (builtin || !selected) return;
    try {
      await api.updatePromptSet(selected.id, name.trim(), system.trim());
      show("Prompt set saved.");
      await reload(selected.id);
    } catch (e) {
      show(msg(e), "error");
    }
  }
  async function duplicate() {
    try {
      const created = await api.createPromptSet(`${name || "Prompt set"} copy`, system || "You are a helpful assistant.");
      show("Duplicated.");
      await reload(created.set?.id);
    } catch (e) {
      show(msg(e), "error");
    }
  }
  async function remove() {
    if (builtin || !selected) return;
    try {
      await api.deletePromptSet(selected.id);
      show("Deleted.");
      await reload("");
    } catch (e) {
      show(msg(e), "error");
    }
  }

  return (
    <div className="group">
      <h3 className="group-title">Prompt set editor</h3>
      <label className="field">
        <span className="label">Edit set</span>
        <select value={selId} disabled={!backendOnline} onChange={(e) => select(e.target.value)}>
          {sets.map((s) => (
            <option key={s.id} value={s.id}>{s.name}{s.builtin ? " (built-in)" : ""}</option>
          ))}
        </select>
      </label>
      <label className="field">
        <span className="label">
          Name {builtin && <span className="badge">Built-in — read-only</span>}
        </span>
        <input type="text" maxLength={80} value={name} readOnly={builtin || locked} onChange={(e) => setName(e.target.value)} />
      </label>
      <label className="field">
        <span className="label">
          Behavior instructions <span className="hint-inline">the motion JSON contract is enforced by code</span>
        </span>
        <textarea rows={6} maxLength={16384} value={system} readOnly={builtin || locked} onChange={(e) => setSystem(e.target.value)} />
      </label>
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void duplicate()}>Duplicate as new</button>
        <button type="button" className="btn btn-primary" disabled={locked || builtin} onClick={() => void save()}>Save set</button>
        <button type="button" className="btn btn-danger-outline" disabled={locked || builtin} onClick={() => void remove()}>Delete set</button>
      </div>
      {locked && <p className="form-status">{backendOnline ? "Read-only client." : "Core offline."}</p>}
    </div>
  );
}
