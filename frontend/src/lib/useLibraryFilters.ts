import { useCallback, useEffect, useMemo, useState } from "react";

export const LIBRARY_PAGE_SIZE = 24;

export type PatternFilterQuery = Record<string, string | number | boolean>;

export function useLibraryFilters() {
  const [zone, setZone] = useState("");
  const [speed, setSpeed] = useState("");
  const [rhythm, setRhythm] = useState("");
  const [strokeLength, setStrokeLength] = useState("");
  const [category, setCategory] = useState("");
  const [minInt, setMinInt] = useState(0);
  const [maxInt, setMaxInt] = useState(100);
  const [minDurationSec, setMinDurationSec] = useState<number | "">("");
  const [maxDurationSec, setMaxDurationSec] = useState<number | "">("");
  const [favOnly, setFavOnly] = useState(false);
  const [userRecordedOnly, setUserRecordedOnly] = useState(false);
  const [minBpm, setMinBpm] = useState<number | "">("");
  const [maxBpm, setMaxBpm] = useState<number | "">("");
  const [sortBy, setSortBy] = useState("");
  const [nameSearch, setNameSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [page, setPage] = useState(1);

  const filterQuery = useCallback((): PatternFilterQuery => {
    return {
      zone,
      speed,
      rhythm,
      stroke_length: strokeLength,
      category,
      min_intensity: minInt,
      max_intensity: maxInt,
      favorites_only: favOnly,
      user_recorded_only: userRecordedOnly,
      ...(minDurationSec !== ""
        ? { min_duration_ms: Math.round(Number(minDurationSec) * 1000) }
        : {}),
      ...(maxDurationSec !== ""
        ? { max_duration_ms: Math.round(Number(maxDurationSec) * 1000) }
        : {}),
      ...(minBpm !== "" ? { min_bpm: minBpm } : {}),
      ...(maxBpm !== "" ? { max_bpm: maxBpm } : {}),
      ...(sortBy ? { sort: sortBy } : {}),
      ...(nameSearch.trim() ? { q: nameSearch.trim() } : {}),
    };
  }, [
    zone,
    speed,
    rhythm,
    strokeLength,
    category,
    minInt,
    maxInt,
    favOnly,
    userRecordedOnly,
    minDurationSec,
    maxDurationSec,
    minBpm,
    maxBpm,
    sortBy,
    nameSearch,
  ]);

  const filters = useCallback(
    () => ({
      ...filterQuery(),
      offset: (page - 1) * LIBRARY_PAGE_SIZE,
      limit: LIBRARY_PAGE_SIZE,
    }),
    [filterQuery, page],
  );

  useEffect(() => {
    setPage(1);
  }, [filterQuery]);

  useEffect(() => {
    const timer = window.setTimeout(() => setNameSearch(searchInput.trim()), 350);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const activeFilterCount = useMemo(() => {
    let count = 0;
    if (zone) count++;
    if (speed) count++;
    if (rhythm) count++;
    if (strokeLength) count++;
    if (category) count++;
    if (minInt > 0) count++;
    if (maxInt < 100) count++;
    if (minDurationSec !== "") count++;
    if (maxDurationSec !== "") count++;
    if (favOnly) count++;
    if (userRecordedOnly) count++;
    if (minBpm !== "") count++;
    if (maxBpm !== "") count++;
    if (sortBy) count++;
    if (nameSearch.trim()) count++;
    return count;
  }, [
    zone,
    speed,
    rhythm,
    strokeLength,
    category,
    minInt,
    maxInt,
    minDurationSec,
    maxDurationSec,
    favOnly,
    userRecordedOnly,
    minBpm,
    maxBpm,
    sortBy,
    nameSearch,
  ]);

  const resetFilters = useCallback(() => {
    setSearchInput("");
    setNameSearch("");
    setZone("");
    setSpeed("");
    setRhythm("");
    setStrokeLength("");
    setCategory("");
    setMinInt(0);
    setMaxInt(100);
    setMinDurationSec("");
    setMaxDurationSec("");
    setFavOnly(false);
    setUserRecordedOnly(false);
    setMinBpm("");
    setMaxBpm("");
    setSortBy("");
    setPage(1);
  }, []);

  return {
    zone,
    setZone,
    speed,
    setSpeed,
    rhythm,
    setRhythm,
    strokeLength,
    setStrokeLength,
    category,
    setCategory,
    minInt,
    setMinInt,
    maxInt,
    setMaxInt,
    minDurationSec,
    setMinDurationSec,
    maxDurationSec,
    setMaxDurationSec,
    favOnly,
    setFavOnly,
    userRecordedOnly,
    setUserRecordedOnly,
    minBpm,
    setMinBpm,
    maxBpm,
    setMaxBpm,
    sortBy,
    setSortBy,
    searchInput,
    setSearchInput,
    page,
    setPage,
    filterQuery,
    filters,
    activeFilterCount,
    resetFilters,
  };
}
