import { formatNumber, t, translateKnown } from "../i18n";
import { useState } from "react";
import { api } from "../api/client";
import type { MediaTagCount } from "../api/types";
import { CheckIcon, CloseIcon, PencilIcon, TrashIcon } from "../shell/icons";
import { VideoDialog } from "./VideoDialog";

interface Props {
  tags: MediaTagCount[];
  locked: boolean;
  onClose: () => void;
  /** A library-wide change touched many rows; the catalog should reload. */
  onChanged: () => void;
}

type Pending = { tag: string; action: "rename" | "delete" } | null;

// Library-wide tag edits. Renaming onto an existing tag merges the two;
// deleting asks once more inside the dialog before removing it everywhere.
export function TagManagerDialog({ tags, locked, onClose, onChanged }: Props) {
  const [editing, setEditing] = useState<{ tag: string; value: string } | null>(null);
  const [confirming, setConfirming] = useState("");
  const [pending, setPending] = useState<Pending>(null);
  const [error, setError] = useState("");

  async function rename(tag: string, value: string) {
    const target = value.trim();
    if (!target || target === tag) {
      setEditing(null);
      return;
    }
    setPending({ tag, action: "rename" });
    setError("");
    try {
      await api.renameMediaTag(tag, target);
      setEditing(null);
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? translateKnown(reason.message) : t("The tag could not be renamed."));
    } finally {
      setPending(null);
    }
  }

  async function remove(tag: string) {
    setPending({ tag, action: "delete" });
    setError("");
    try {
      await api.deleteMediaTag(tag);
      setConfirming("");
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? translateKnown(reason.message) : t("The tag could not be removed."));
    } finally {
      setPending(null);
    }
  }

  const busy = pending !== null;
  return (
    <VideoDialog
      titleID="video-tag-manager-title"
      title={t("Manage tags")}
      busy={busy}
      className="video-tag-manager"
      onClose={onClose}
      footer={<button type="button" className="btn btn-secondary" data-autofocus disabled={busy} onClick={onClose}>{t("Done")}</button>}
    >
      {tags.length === 0 ? (
        <p className="hint">{t("No tags yet. Add tags from a video's details.")}</p>
      ) : (
        <ul className="video-tag-manager-list">
          {tags.map((entry) => (
            <li key={entry.tag.toLocaleLowerCase()}>
              {editing?.tag === entry.tag ? (
                <form className="video-tag-rename" onSubmit={(event) => { event.preventDefault(); void rename(entry.tag, editing.value); }}>
                  <label className="visually-hidden" htmlFor="video-tag-rename-input">{t("New name for {tag}", { tag: entry.tag })}</label>
                  <input id="video-tag-rename-input" autoFocus value={editing.value} maxLength={40} disabled={busy} onChange={(event) => setEditing({ tag: entry.tag, value: event.target.value })} />
                  <button type="submit" className="icon-button" aria-label={t("Save name")} title={t("Save name")} disabled={busy}><CheckIcon size={16} /></button>
                  <button type="button" className="icon-button" aria-label={t("Cancel rename")} title={t("Cancel rename")} disabled={busy} onClick={() => setEditing(null)}><CloseIcon size={16} /></button>
                </form>
              ) : (
                <>
                  <span className="video-tag">{entry.tag}</span>
                  <span className="video-tag-count">{entry.count === 1 ? t("1 video") : t("{count} videos", { count: formatNumber(entry.count) })}</span>
                  {confirming === entry.tag ? (
                    <span className="video-tag-confirm">
                      <button type="button" className="btn btn-secondary compact-command" disabled={busy || locked} onClick={() => void remove(entry.tag)}>{t("Remove from {count} videos", { count: formatNumber(entry.count) })}</button>
                      <button type="button" className="btn btn-secondary compact-command" disabled={busy} onClick={() => setConfirming("")}>{t("Keep")}</button>
                    </span>
                  ) : (
                    <span className="video-tag-actions">
                      <button type="button" className="icon-button" aria-label={t("Rename {tag}", { tag: entry.tag })} title={t("Rename {tag}", { tag: entry.tag })} disabled={busy || locked} onClick={() => { setConfirming(""); setEditing({ tag: entry.tag, value: entry.tag }); }}><PencilIcon size={16} /></button>
                      <button type="button" className="icon-button" aria-label={t("Remove {tag}", { tag: entry.tag })} title={t("Remove {tag}", { tag: entry.tag })} disabled={busy || locked} onClick={() => { setEditing(null); setConfirming(entry.tag); }}><TrashIcon size={16} /></button>
                    </span>
                  )}
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      <p className="hint">{t("Renaming a tag to one that already exists merges them.")}</p>
      {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
    </VideoDialog>
  );
}
