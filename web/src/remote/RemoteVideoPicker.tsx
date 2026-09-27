import { t, translateKnown } from "../i18n";
import { useMemo, useState } from "react";
import { filterVideos, noVideoFilters, sortVideos, videoTitle } from "../videos/curation";
import { formatDuration } from "../videos/format";
import { useMediaCatalog } from "../videos/useMediaCatalog";
import type { RemoteSend } from "./useRemoteCommands";

// Long libraries stay searchable without rendering every row on a phone.
const SHOWN_LIMIT = 60;

// Opens a catalog video on the desktop. Playback starts only when the phone
// presses play, so opening never moves the device by itself.
export function RemoteVideoPicker({ currentID, send }: { currentID?: string; send: RemoteSend }) {
  const catalog = useMediaCatalog();
  const [query, setQuery] = useState("");
  const matches = useMemo(() => sortVideos(
    filterVideos(catalog.videos.filter((video) => !video.missing), { ...noVideoFilters, query }),
    "name",
  ), [catalog.videos, query]);
  const shown = matches.slice(0, SHOWN_LIMIT);

  return (
    <section className="remote-picker" aria-label={t("Open a video on the desktop")}>
      <h2 className="section-title">{t("Open a video")}</h2>
      <input
        type="search"
        className="remote-picker-search"
        value={query}
        placeholder={t("Search titles, tags and notes")}
        aria-label={t("Search videos")}
        onChange={(event) => setQuery(event.target.value)}
      />
      {catalog.error ? (
        <p className="form-status" role="alert">{translateKnown(catalog.error)}</p>
      ) : catalog.loading && catalog.videos.length === 0 ? (
        <p className="form-status" role="status">{t("Loading videos")}</p>
      ) : shown.length === 0 ? (
        <p className="form-status" role="status">{query.trim() ? t("No videos match the filters.") : t("No videos yet")}</p>
      ) : (
        <ul className="remote-picker-list">
          {shown.map((video) => (
            <li key={video.id}>
              <button
                type="button"
                className="remote-picker-item"
                aria-current={video.id === currentID || undefined}
                onClick={() => void send({ target: "video", action: "open", video_id: video.id })}
              >
                <span className="remote-picker-title">{videoTitle(video)}</span>
                <span className="remote-picker-meta">
                  {translateKnown(formatDuration(video.duration_ms))}
                  {video.has_funscript && <span className="remote-picker-script">{t("Script")}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {matches.length > shown.length && (
        <p className="form-status">{t("Showing {shown} of {count}. Search to narrow the list.", { shown: shown.length, count: matches.length })}</p>
      )}
    </section>
  );
}
