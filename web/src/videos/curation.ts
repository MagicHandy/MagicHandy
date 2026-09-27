import type { MediaTagCount, MediaVideo } from "../api/types";

/** What the library shows for a video: the user's title, else the file name. */
export function videoTitle(video: MediaVideo): string {
  return video.title?.trim() || video.display_name;
}

export type ScriptFilter = "any" | "paired" | "unpaired";
export type VideoSort = "name" | "recent" | "rating";

export interface VideoFilters {
  query: string;
  /** Every listed tag must be present. */
  tags: string[];
  /** 0 accepts unrated videos. */
  minimumRating: number;
  script: ScriptFilter;
}

export const noVideoFilters: VideoFilters = { query: "", tags: [], minimumRating: 0, script: "any" };

export function filtersActive(filters: VideoFilters): boolean {
  return filters.query.trim() !== "" || filters.tags.length > 0 || filters.minimumRating > 0 || filters.script !== "any";
}

export function filterVideos(videos: MediaVideo[], filters: VideoFilters): MediaVideo[] {
  const needle = filters.query.trim().toLocaleLowerCase();
  const wanted = filters.tags.map((tag) => tag.toLocaleLowerCase());
  return videos.filter((video) => {
    if (needle && !searchText(video).includes(needle)) return false;
    if (wanted.length > 0) {
      const tags = new Set((video.tags ?? []).map((tag) => tag.toLocaleLowerCase()));
      if (!wanted.every((tag) => tags.has(tag))) return false;
    }
    if (filters.minimumRating > 0 && (video.rating ?? 0) < filters.minimumRating) return false;
    if (filters.script === "paired" && !video.has_funscript) return false;
    if (filters.script === "unpaired" && video.has_funscript) return false;
    return true;
  });
}

function searchText(video: MediaVideo): string {
  return [video.title ?? "", video.display_name, video.location_path, ...(video.tags ?? []), video.notes ?? ""]
    .join(" ")
    .toLocaleLowerCase();
}

/** Unavailable entries always sort after playable ones. */
export function sortVideos(videos: MediaVideo[], sort: VideoSort): MediaVideo[] {
  return [...videos].sort((left, right) => {
    const availability = Number(left.missing) - Number(right.missing);
    if (availability !== 0) return availability;
    const byName = videoTitle(left).localeCompare(videoTitle(right), undefined, { sensitivity: "base" });
    if (sort === "recent") return Date.parse(right.modified_at) - Date.parse(left.modified_at) || byName;
    if (sort === "rating") return (right.rating ?? 0) - (left.rating ?? 0) || byName;
    return byName;
  });
}

/** Counts the tags in a loaded catalog, in the library's own spelling. */
export function tagCounts(videos: MediaVideo[]): MediaTagCount[] {
  const counts = new Map<string, MediaTagCount>();
  for (const video of videos) {
    for (const tag of video.tags ?? []) {
      const key = tag.toLocaleLowerCase();
      const entry = counts.get(key);
      if (entry) entry.count += 1;
      else counts.set(key, { tag, count: 1 });
    }
  }
  return [...counts.values()].sort((left, right) => left.tag.localeCompare(right.tag, undefined, { sensitivity: "base" }));
}

/** Splits typed tag text on commas; the server normalizes and bounds tags. */
export function splitTagInput(value: string): string[] {
  return value.split(",").map((tag) => tag.replace(/\s+/g, " ").trim()).filter(Boolean);
}

/** Reuses the library's spelling of a tag that already exists. */
export function canonicalTag(tag: string, known: readonly string[]): string {
  const key = tag.toLocaleLowerCase();
  return known.find((candidate) => candidate.toLocaleLowerCase() === key) ?? tag;
}
