import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ElevenLabsLibraryVoice, ElevenLabsVoice } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { trapModalTab } from "../util/modal";

export interface ChosenVoice {
  voice_id: string;
  name: string;
  labels?: Record<string, string> | null;
  preview_url?: string | null;
}

interface Props {
  currentVoiceID: string;
  // false: confirmed free plan; undefined: plan unknown (the key cannot read it).
  libraryViaAPI?: boolean;
  canAddVoices: boolean;
  onClose: () => void;
  onChoose: (voice: ChosenVoice) => void;
  onPreview: (url?: string | null) => void;
}

type Tab = "mine" | "library";
type MineFilter = "" | "default" | "personal" | "community";

// Suggested searches tuned for a companion voice that speaks to one listener.
// Each is an ordinary Voice Library query; nothing is hard-coded per voice.
const PRESETS: { id: string; label: MessageKey; query: Record<string, string> }[] = [
  { id: "warm", label: "Warm & close", query: { search: "intimate" } },
  { id: "soft", label: "Soft & breathy", query: { search: "soft" } },
  { id: "playful", label: "Playful", query: { search: "playful" } },
  { id: "low", label: "Low & steady", query: { search: "deep" } },
  { id: "asmr", label: "ASMR", query: { search: "asmr" } },
  { id: "conversational", label: "Conversational", query: { use_cases: "conversational" } },
];

const MINE_FILTERS: { id: MineFilter; label: MessageKey }[] = [
  { id: "", label: "All" },
  { id: "default", label: "Default" },
  { id: "personal", label: "Made by you" },
  { id: "community", label: "From the library" },
];

function words(value?: string | null): string {
  return (value ?? "").replace(/_/g, " ").trim();
}

function libraryMeta(voice: ElevenLabsLibraryVoice): string {
  return [voice.gender, voice.age, voice.accent, voice.descriptive, voice.use_case].map(words).filter(Boolean).join(" · ");
}

function voiceMeta(voice: ElevenLabsVoice): string {
  const labels = voice.labels ?? {};
  return [labels.gender, labels.age, labels.accent, labels.descriptive, labels.use_case].map(words).filter(Boolean).join(" · ");
}

function message(error: unknown): string {
  return error instanceof Error ? translateKnown(error.message) : t("Request failed");
}

export function ElevenLabsVoiceBrowser({ currentVoiceID, libraryViaAPI, canAddVoices, onClose, onChoose, onPreview }: Props) {
  const dialogRef = useRef<HTMLElement>(null);
  const [tab, setTab] = useState<Tab>("mine");
  const [search, setSearch] = useState("");
  const [mineFilter, setMineFilter] = useState<MineFilter>("");
  const [preset, setPreset] = useState("warm");
  const [gender, setGender] = useState("");
  const [age, setAge] = useState("");
  const [language, setLanguage] = useState("en");
  const [sort, setSort] = useState("trending");
  const [hideModerated, setHideModerated] = useState(true);
  const [hideCustomRates, setHideCustomRates] = useState(true);
  const [mine, setMine] = useState<ElevenLabsVoice[]>([]);
  const [library, setLibrary] = useState<ElevenLabsLibraryVoice[]>([]);
  const [more, setMore] = useState<{ token?: string; page?: number } | null>(null);
  const [loading, setLoading] = useState(false);
  const [failure, setFailure] = useState("");
  const [pending, setPending] = useState<ChosenVoice | null>(null);
  const [adding, setAdding] = useState("");
  const freePlan = libraryViaAPI === false;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialogRef.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
      else if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      previous?.focus?.();
    };
  }, [onClose]);

  const libraryQuery = (page: number): Record<string, string> => {
    const base = search.trim() ? { search: search.trim() } : PRESETS.find((item) => item.id === preset)?.query ?? {};
    const query: Record<string, string> = { ...base, sort, page_size: "30" };
    if (gender) query.gender = gender;
    if (age) query.age = age;
    if (language) query.language = language;
    if (!hideModerated) query.include_live_moderated = "true";
    if (!hideCustomRates) query.include_custom_rates = "true";
    if (page > 0) query.page = String(page);
    return query;
  };
  const mineQuery = (token?: string): Record<string, string> => {
    const query: Record<string, string> = { page_size: "30", sort: "name" };
    if (search.trim()) query.search = search.trim();
    if (mineFilter) query.type = mineFilter;
    if (token) query.page_token = token;
    return query;
  };

  // Results follow the filters; typing waits a moment before searching.
  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      setLoading(true);
      setFailure("");
      try {
        if (tab === "mine") {
          const page = await api.elevenLabsVoices(mineQuery(), controller.signal);
          setMine(page.voices ?? []);
          setMore(page.has_more && page.next_page_token ? { token: page.next_page_token } : null);
        } else {
          const page = await api.elevenLabsLibrary(libraryQuery(0), controller.signal);
          setLibrary(page.voices ?? []);
          setMore(page.has_more ? { page: 1 } : null);
        }
      } catch (error) {
        if (!controller.signal.aborted) setFailure(message(error));
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    }, search ? 300 : 0);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
    // The query builders read exactly these inputs.
  }, [tab, search, mineFilter, preset, gender, age, language, sort, hideModerated, hideCustomRates]);

  async function loadMore() {
    if (!more) return;
    setLoading(true);
    try {
      if (tab === "mine" && more.token) {
        const page = await api.elevenLabsVoices(mineQuery(more.token));
        setMine((current) => [...current, ...(page.voices ?? [])]);
        setMore(page.has_more && page.next_page_token ? { token: page.next_page_token } : null);
      } else if (tab === "library" && more.page) {
        const page = await api.elevenLabsLibrary(libraryQuery(more.page));
        setLibrary((current) => [...current, ...(page.voices ?? [])]);
        setMore(page.has_more ? { page: more.page + 1 } : null);
      }
    } catch (error) {
      setFailure(message(error));
    } finally {
      setLoading(false);
    }
  }

  async function addAndUse(voice: ElevenLabsLibraryVoice) {
    setAdding(voice.voice_id);
    setFailure("");
    try {
      const added = await api.elevenLabsAddVoice({ public_owner_id: voice.public_owner_id, voice_id: voice.voice_id, name: voice.name });
      onChoose({
        voice_id: added.voice_id,
        name: voice.name,
        labels: { gender: voice.gender ?? "", age: voice.age ?? "", accent: voice.accent ?? "", descriptive: voice.descriptive ?? "", use_case: voice.use_case ?? "" },
        preview_url: voice.preview_url,
      });
    } catch (error) {
      setFailure(message(error));
    } finally {
      setAdding("");
    }
  }

  const switchTab = (next: Tab) => {
    setTab(next);
    setSearch("");
    setMore(null);
  };

  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section ref={dialogRef} className="reference-dialog elevenlabs-browser" role="dialog" aria-modal="true" aria-labelledby="elevenlabs-browser-title" tabIndex={-1}>
        <header className="reference-dialog-header">
          <h2 id="elevenlabs-browser-title">{t("Choose a voice")}</h2>
          <button type="button" className="icon-button" aria-label={t("Close")} onClick={onClose}>
            <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" /></svg>
          </button>
        </header>

        <div className="elevenlabs-browser-tabs">
          <div className="elevenlabs-tabs" role="tablist" aria-label={t("Voice sources")}>
            <button type="button" role="tab" aria-selected={tab === "mine"} onClick={() => switchTab("mine")}>{t("Your voices")}</button>
            <button type="button" role="tab" aria-selected={tab === "library"} onClick={() => switchTab("library")}>{t("Voice Library")}</button>
          </div>
        </div>

        <div className="elevenlabs-browser-body">
          <div className="field-action-row">
            <input type="search" aria-label={tab === "mine" ? t("Search your voices") : t("Search the Voice Library")} placeholder={tab === "mine" ? t("Search your voices") : t("Search by name, mood or accent")} value={search} onChange={(event) => setSearch(event.target.value)} />
            {tab === "library" && (
              <select aria-label={t("Sort")} value={sort} onChange={(event) => setSort(event.target.value)}>
                <option value="trending">{t("Trending")}</option>
                <option value="usage_character_count_1y">{t("Most used this year")}</option>
                <option value="created_date">{t("Newest")}</option>
              </select>
            )}
          </div>

          {tab === "mine" ? (
            <div className="segmented" role="group" aria-label={t("Show")}>
              {MINE_FILTERS.map((item) => <button key={item.id || "all"} type="button" aria-pressed={mineFilter === item.id} onClick={() => setMineFilter(item.id)}>{translateKnown(item.label)}</button>)}
            </div>
          ) : (
            <>
              {freePlan && (
                <p className="voice-inset-note voice-inset-note-warn">
                  <strong>{t("Library voices need a paid plan here.")}</strong>{" "}
                  {t("ElevenLabs only lets paid plans (Starter or higher) use Voice Library voices through apps like MagicHandy. Previews are free, and your own and default voices work on every plan.")}
                </p>
              )}
              <div className="elevenlabs-presets" role="group" aria-label={t("Suggested searches")}>
                {PRESETS.map((item) => (
                  <button key={item.id} type="button" className="elevenlabs-preset" aria-pressed={!search.trim() && preset === item.id} onClick={() => { setSearch(""); setPreset(item.id); }}>{translateKnown(item.label)}</button>
                ))}
              </div>
              <div className="elevenlabs-filters">
                <select aria-label={t("Gender")} value={gender} onChange={(event) => setGender(event.target.value)}>
                  <option value="">{t("Any gender")}</option><option value="female">{t("Female")}</option><option value="male">{t("Male")}</option><option value="neutral">{t("Neutral")}</option>
                </select>
                <select aria-label={t("Age")} value={age} onChange={(event) => setAge(event.target.value)}>
                  <option value="">{t("Any age")}</option><option value="young">{t("Young")}</option><option value="middle_aged">{t("Middle-aged")}</option><option value="old">{t("Old")}</option>
                </select>
                <select aria-label={t("Language")} value={language} onChange={(event) => setLanguage(event.target.value)}>
                  <option value="en">{t("English")}</option><option value="">{t("Any language")}</option>
                </select>
                <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={hideModerated} onChange={(event) => setHideModerated(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Hide voices that screen what they say")}</span></label>
                <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={hideCustomRates} onChange={(event) => setHideCustomRates(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Hide voices that cost extra credits")}</span></label>
              </div>
            </>
          )}

          {failure && <p className="voice-status-error" role="alert">{failure}</p>}
          <ul className="elevenlabs-voice-list" aria-busy={loading}>
            {tab === "mine" && mine.map((voice) => {
              const selected = (pending?.voice_id ?? currentVoiceID) === voice.voice_id;
              return (
                <li key={voice.voice_id} className="elevenlabs-voice-item" data-selected={selected || undefined}>
                  <button type="button" className="btn btn-secondary btn-sm elevenlabs-play" disabled={!voice.preview_url} aria-label={t("Play a preview of {name}", { name: voice.name })} onClick={() => onPreview(voice.preview_url)}>
                    <svg width="12" height="12" viewBox="0 0 14 14" aria-hidden="true"><path d="M3.5 2.2v9.6L11.5 7z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" /></svg>
                  </button>
                  <span className="elevenlabs-voice-text"><span className="elevenlabs-voice-name">{voice.name}</span><span className="hint">{voiceMeta(voice)}</span></span>
                  {voice.is_legacy && <span className="badge" title={t("An older default voice that ElevenLabs may retire.")}>{t("Legacy")}</span>}
                  {selected
                    ? <span className="elevenlabs-selected">{t("Selected")}</span>
                    : <button type="button" className="btn btn-secondary btn-sm" onClick={() => setPending({ voice_id: voice.voice_id, name: voice.name, labels: voice.labels, preview_url: voice.preview_url })}>{t("Use")}</button>}
                </li>
              );
            })}
            {tab === "library" && library.map((voice) => (
              <li key={`${voice.public_owner_id}/${voice.voice_id}`} className="elevenlabs-voice-item">
                <button type="button" className="btn btn-secondary btn-sm elevenlabs-play" disabled={!voice.preview_url} aria-label={t("Play a preview of {name}", { name: voice.name })} onClick={() => onPreview(voice.preview_url)}>
                  <svg width="12" height="12" viewBox="0 0 14 14" aria-hidden="true"><path d="M3.5 2.2v9.6L11.5 7z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" /></svg>
                </button>
                <span className="elevenlabs-voice-text"><span className="elevenlabs-voice-name">{voice.name}</span><span className="hint">{libraryMeta(voice)}</span></span>
                {voice.category === "high_quality" && <span className="badge">{t("Studio quality")}</span>}
                {voice.live_moderation_enabled && <span className="badge">{t("Screens what it says")}</span>}
                {(voice.rate ?? 1) > 1 && <span className="badge">{t("{rate}× credits", { rate: voice.rate ?? 1 })}</span>}
                {voice.is_added_by_user
                  ? <span className="elevenlabs-selected">{t("In your voices")}</span>
                  : freePlan
                    ? <span className="hint">{t("Needs a paid plan")}</span>
                    : <button type="button" className="btn btn-secondary btn-sm" disabled={!canAddVoices || Boolean(adding)} onClick={() => void addAndUse(voice)}>{adding === voice.voice_id ? t("Adding…") : t("Add & use")}</button>}
              </li>
            ))}
          </ul>
          {!loading && !failure && (tab === "mine" ? mine : library).length === 0 && <p className="hint">{t("No voices match.")}</p>}
          {more && <button type="button" className="btn btn-secondary btn-sm elevenlabs-more" disabled={loading} onClick={() => void loadMore()}>{t("Load more")}</button>}
          <p className="hint">{tab === "mine"
            ? t("Default voices work on every plan. Voices you create or save in ElevenLabs appear here.")
            : t("Previews are free. Adding a voice saves it to your ElevenLabs account; library voices don't use a custom voice slot.")}</p>
        </div>

        <footer className="reference-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>{t("Cancel")}</button>
          <button type="button" className="btn btn-primary" disabled={!pending || pending.voice_id === currentVoiceID} onClick={() => pending && onChoose(pending)}>{t("Use voice")}</button>
        </footer>
      </section>
    </div>
  );
}
