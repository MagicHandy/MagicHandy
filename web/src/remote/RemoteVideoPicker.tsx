import { t, translateKnown } from "../i18n";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { RemoteVideoItem } from "../api/remote-types";
import { formatDuration } from "../videos/format";
import { VideoIcon } from "../shell/icons";
import type { RemoteSend } from "./useRemoteCommands";

// Display-only, bounded catalog pages: no file paths or private metadata.
export function RemoteVideoPicker({ currentID, send }: { currentID?: string; send: RemoteSend }) {
  const [query,setQuery] = useState("");
  const [offset,setOffset] = useState(0);
  const [items,setItems] = useState<RemoteVideoItem[]>([]);
  const [next,setNext] = useState<number | null>(null);
  const [loading,setLoading] = useState(false);
  const [error,setError] = useState("");
  useEffect(() => {
    const abort = new AbortController();
    setLoading(true);
    setError("");
    const timer=setTimeout(() => {
      void api.remoteVideos(query,offset,abort.signal).then(page => {
        if (abort.signal.aborted) return;
        setItems(previous => offset === 0 ? page.videos : [...previous,...page.videos.filter(item => !previous.some(existing => existing.id === item.id))]);
        setNext(page.has_more ? page.next_offset : null);
      }, reason => {
        if (!abort.signal.aborted) setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
      }).finally(() => { if (!abort.signal.aborted) setLoading(false); });
    },query && offset === 0 ? 200 : 0);
    return () => { clearTimeout(timer);abort.abort(); };
  },[query,offset]);
  return <section className="remote-picker" aria-label={t("Open a video on the desktop")}>
    <div className="remote-section-heading"><h2>{t("Video library")}</h2>
    <input type="search" className="remote-picker-search" value={query} placeholder={t("Search titles and tags")} aria-label={t("Search videos")}
      onChange={event => { setQuery(event.target.value);setOffset(0);setItems([]);setNext(null); }} />
    </div>
    {error && <p className="form-status" role="alert">{error}</p>}
    {loading && <p className="form-status" role="status">{t("Loading videos")}</p>}
    {!error && !loading && items.length === 0 && <p className="form-status">{query ? t("No videos match the filters.") : t("No videos yet")}</p>}
    <ul className="remote-picker-list">{items.map(video => <li key={video.id}>
      <button type="button" className="remote-picker-item" aria-current={video.id === currentID || undefined} onClick={() => void send({ target:"video",action:"open",video_id:video.id })}>
        <span className="remote-picker-symbol" aria-hidden="true"><VideoIcon size={22} /></span>
        <span className="remote-picker-title">{video.title}</span><span className="remote-picker-meta">{translateKnown(formatDuration(video.duration_ms))}{video.has_funscript && <span>{t("Script")}</span>}</span>
      </button>
    </li>)}</ul>
    {next !== null && <button className="btn btn-secondary small" type="button" disabled={loading} onClick={() => setOffset(next)}>{t("Show more")}</button>}
  </section>;
}
