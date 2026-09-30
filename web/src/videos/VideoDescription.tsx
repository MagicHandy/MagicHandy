import { t } from "../i18n";
import type { MediaVideo } from "../api/types";
import { RatingStars, TagChips } from "./CurationControls";
import { formatFileSize, formatLocation } from "./format";

// Everything about the open video beyond its title, folded away below the
// player and its timeline so the picture leads. It starts closed each time a
// video opens.
export function VideoDescription({ video, title }: { video: MediaVideo; title: string }) {
  const file = video.has_funscript
    ? t("{size} / {location} / script found", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) })
    : t("{size} / {location}", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) });
  const curated = Boolean(video.rating) || (video.tags?.length ?? 0) > 0;
  return (
    <details className="video-description">
      <summary>{t("Details")}</summary>
      <div className="video-description-body">
        {curated && (
          <div className="video-description-curation">
            <RatingStars rating={video.rating} />
            <TagChips tags={video.tags} limit={video.tags?.length ?? 0} />
          </div>
        )}
        {video.notes && <p className="video-description-notes">{video.notes}</p>}
        <p className="video-description-file">
          {title !== video.display_name && <span>{video.display_name}</span>}
          <span>{file}</span>
        </p>
      </div>
    </details>
  );
}
