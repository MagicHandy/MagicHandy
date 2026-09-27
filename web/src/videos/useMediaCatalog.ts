import { t } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";

export interface MediaCatalog {
  videos: MediaVideo[];
  loading: boolean;
  error: string;
  reload: (signal?: AbortSignal) => Promise<void>;
  /** Replaces one row with a newer server copy without a catalog reload. */
  replaceVideo: (video: MediaVideo) => void;
  replaceVideos: (videos: MediaVideo[]) => void;
}

// The catalog is a snapshot of the SQLite rows. Reloading it is distinct from
// scanning the filesystem, and overlapping reads cannot let an older response
// replace a newer one.
export function useMediaCatalog(): MediaCatalog {
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const loadGeneration = useRef(0);

  const reload = useCallback(async (signal?: AbortSignal) => {
    const generation = ++loadGeneration.current;
    const current = () => !signal?.aborted && mounted.current && generation === loadGeneration.current;
    setLoading(true);
    setError("");
    try {
      const response = await api.mediaVideos(signal);
      if (current()) setVideos(Array.isArray(response.videos) ? response.videos : []);
    } catch (reason) {
      if (current()) setError(reason instanceof Error ? reason.message : t("Video catalog could not be loaded."));
    } finally {
      if (current()) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void reload(controller.signal);
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
      controller.abort();
    };
  }, [reload]);

  const replaceVideo = useCallback((video: MediaVideo) => {
    setVideos((current) => current.map((entry) => entry.id === video.id ? video : entry));
  }, []);

  const replaceVideos = useCallback((updated: MediaVideo[]) => {
    const byID = new Map(updated.map((video) => [video.id, video]));
    setVideos((current) => current.map((entry) => byID.get(entry.id) ?? entry));
  }, []);

  return { videos, loading, error, reload, replaceVideo, replaceVideos };
}
