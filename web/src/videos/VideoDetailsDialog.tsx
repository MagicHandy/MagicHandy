import { t, translateKnown } from "../i18n";
import { useState } from "react";
import { api } from "../api/client";
import type { MediaMetadataPatch, MediaVideo } from "../api/types";
import { RatingInput, TagEditor } from "./CurationControls";
import { VideoDialog } from "./VideoDialog";

interface Props {
  video: MediaVideo;
  tagSuggestions: readonly string[];
  onClose: () => void;
  onSaved: (video: MediaVideo) => void;
}

// Title, rating, tags and notes for one video. Only changed fields are sent,
// so two people editing different fields do not overwrite each other.
export function VideoDetailsDialog({ video, tagSuggestions, onClose, onSaved }: Props) {
  const [title, setTitle] = useState(video.title ?? "");
  const [rating, setRating] = useState(video.rating ?? 0);
  const [tags, setTags] = useState<string[]>(video.tags ?? []);
  const [notes, setNotes] = useState(video.notes ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  function changes(): MediaMetadataPatch {
    const patch: MediaMetadataPatch = {};
    if (title.trim() !== (video.title ?? "")) patch.title = title.trim();
    if (rating !== (video.rating ?? 0)) patch.rating = rating;
    if (notes.trim() !== (video.notes ?? "")) patch.notes = notes.trim();
    if (tags.join("\n") !== (video.tags ?? []).join("\n")) patch.tags = tags;
    return patch;
  }

  async function save() {
    const patch = changes();
    if (Object.keys(patch).length === 0) {
      onClose();
      return;
    }
    setSaving(true);
    setError("");
    try {
      const response = await api.saveMediaMetadata(video.id, patch);
      onSaved(response.video);
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? translateKnown(reason.message) : t("Video details could not be saved."));
      setSaving(false);
    }
  }

  return (
    <VideoDialog
      titleID="video-details-title"
      title={t("Video details")}
      busy={saving}
      className="video-details-dialog"
      onClose={onClose}
      footer={(
        <>
          <button type="button" className="btn btn-secondary" disabled={saving} onClick={onClose}>{t("Cancel")}</button>
          <button type="button" className="btn btn-primary" disabled={saving} onClick={() => void save()}>{saving ? t("Saving…") : t("Save")}</button>
        </>
      )}
    >
      <form className="video-details-form" onSubmit={(event) => { event.preventDefault(); void save(); }}>
        <div className="field">
          <label className="label" htmlFor="video-details-title-input">{t("Title")}</label>
          <input
            id="video-details-title-input"
            data-autofocus
            value={title}
            maxLength={200}
            placeholder={video.display_name}
            disabled={saving}
            aria-describedby="video-details-title-hint"
            onChange={(event) => setTitle(event.target.value)}
          />
          <span className="hint" id="video-details-title-hint">{t("File name: {name}", { name: video.display_name })}</span>
        </div>
        <div className="field">
          <span className="label">{t("Rating")}</span>
          <RatingInput value={rating} disabled={saving} onChange={setRating} />
        </div>
        <div className="field">
          <label className="label" htmlFor="video-details-tags">{t("Tags")}</label>
          <TagEditor inputID="video-details-tags" tags={tags} suggestions={tagSuggestions} disabled={saving} onChange={setTags} />
          <span className="hint">{t("Separate tags with commas. Tags are stored in MagicHandy, never in the video file.")}</span>
        </div>
        <label className="field" htmlFor="video-details-notes">
          <span className="label">{t("Notes")}</span>
          <textarea id="video-details-notes" rows={4} maxLength={2000} value={notes} disabled={saving} onChange={(event) => setNotes(event.target.value)} />
        </label>
        {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
      </form>
    </VideoDialog>
  );
}
