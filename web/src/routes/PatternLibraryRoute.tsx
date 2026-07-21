import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { api } from "../api/client";
import type { LibraryPattern, PatternInput, PatternLibrary, PatternPreview } from "../api/types";
import { MotionImport } from "../components/MotionImport";
import { PatternAuthoring } from "../components/PatternAuthoring";
import { PatternBrowser } from "../components/PatternBrowser";
import { PatternTraining } from "../components/PatternTraining";
import { ProgramLibrary } from "../components/ProgramLibrary";
import { libraryActionKey } from "../components/library-actions";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

type View = "browse" | "programs" | "import" | "author" | "training";

const views: readonly View[] = ["browse", "programs", "import", "author", "training"];
const emptyLibrary: PatternLibrary = { patterns: [], programs: [], feedback: [], auto_disable: false };

export function PatternLibraryRoute() {
  const { backendOnline, readOnly, motion, state, refresh } = useAppState();
  const { show } = useToast();
  const [view, setView] = useState<View>("browse");
  const [library, setLibrary] = useState<PatternLibrary>(emptyLibrary);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [busyKeys, setBusyKeys] = useState<ReadonlySet<string>>(() => new Set());
  const inFlightActions = useRef(new Set<string>());
  const loadGeneration = useRef(0);
  const mounted = useRef(true);
  const tabRefs = useRef<Partial<Record<View, HTMLButtonElement | null>>>({});
  const locked = !backendOnline || readOnly;
  const maxIntensity = state?.settings?.motion?.speed_max_percent ?? 100;

  const load = useCallback(async (signal?: AbortSignal) => {
    const generation = ++loadGeneration.current;
    setLoading(true);
    setLoadError("");
    try {
      const response = await api.getLibrary(signal);
      if (generation === loadGeneration.current && !signal?.aborted) {
        setLibrary(normalizeLibrary(response?.library));
      }
    } catch (error) {
      if (generation === loadGeneration.current && !signal?.aborted) {
        setLoadError(error instanceof Error ? error.message : "Pattern library could not be loaded.");
      }
    } finally {
      if (generation === loadGeneration.current && !signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void load(controller.signal);
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
      controller.abort();
    };
  }, [load]);

  function replacePattern(pattern: LibraryPattern) {
    setLibrary((current) => ({
      ...current,
      patterns: current.patterns.map((item) => item.id === pattern.id ? pattern : item),
    }));
  }

  async function patchPattern(id: string, patch: Partial<Pick<LibraryPattern, "name" | "enabled" | "weight">>): Promise<boolean> {
    return withBusy(libraryActionKey.pattern(id), async () => {
      const response = await api.patchPattern(id, patch);
      replacePattern(response.pattern);
    });
  }

  async function playPattern(id: string, intensity = Math.min(30, maxIntensity), feel = "original") {
    await withBusy(libraryActionKey.motionStart, async () => {
      await api.playPattern(id, intensity, feel);
      refresh();
    });
  }

  async function ratePattern(id: string, rating: -1 | 1) {
    await withBusy(libraryActionKey.pattern(id), async () => {
      const response = await api.patternFeedback(id, rating);
      replacePattern(response.pattern);
      setLibrary((current) => ({ ...current, feedback: [response.feedback, ...current.feedback] }));
    });
  }

  async function undoFeedback(id: number) {
    const feedback = library.feedback.find((item) => item.id === id);
    const actionKey = feedback ? libraryActionKey.pattern(feedback.pattern_id) : libraryActionKey.feedback(id);
    await withBusy(actionKey, async () => {
      const response = await api.undoPatternFeedback(id);
      replacePattern(response.pattern);
      setLibrary((current) => ({
        ...current,
        feedback: current.feedback.map((item) => item.id === id ? response.feedback : item),
      }));
    });
  }

  async function savePattern(input: PatternInput): Promise<boolean> {
    return withBusy(libraryActionKey.author, async () => {
      const response = await api.createPattern(input);
      setLibrary((current) => ({ ...current, patterns: [...current.patterns, response.pattern] }));
      setView("browse");
      show(`${response.pattern.name} saved.`);
    });
  }

  async function previewPattern(input: PatternInput, signal?: AbortSignal): Promise<PatternPreview> {
    return (await api.previewPattern(input, signal)).preview;
  }

  function showPreviewError(error: unknown) {
    show(error instanceof Error ? error.message : "Pattern preview failed.", "error");
  }

  async function importFile(file: File, asKind: "pattern" | "program"): Promise<boolean> {
    return withBusy(libraryActionKey.import, async () => {
      const response = await api.importMotionContent(file, asKind);
      const imported = response?.import;
      if (!imported?.pattern && !imported?.program) throw new Error("The import response did not contain motion content.");
      const importedPattern = imported.pattern;
      const importedProgram = imported.program;
      setLibrary((current) => ({
        ...current,
        patterns: importedPattern
          ? [...current.patterns.filter((item) => item.id !== importedPattern.id), importedPattern]
          : current.patterns,
        programs: importedProgram
          ? [...current.programs.filter((item) => item.id !== importedProgram.id), importedProgram]
          : current.programs,
      }));
      const stripped = imported.gaps_stripped > 0 ? ` ${imported.gaps_stripped} long gaps removed.` : "";
      show(`${file.name} imported.${stripped}`);
      setView(importedPattern ? "browse" : "programs");
    });
  }

  async function removePattern(id: string) {
    const pattern = library.patterns.find((item) => item.id === id);
    if (!pattern || !window.confirm(`Delete ${pattern.name}?`)) return;
    await withBusy(libraryActionKey.pattern(id), async () => {
      await api.deletePattern(id);
      setLibrary((current) => ({ ...current, patterns: current.patterns.filter((item) => item.id !== id) }));
    });
  }

  async function removeProgram(id: string) {
    const program = library.programs.find((item) => item.id === id);
    if (!program || !window.confirm(`Delete ${program.name}?`)) return;
    await withBusy(libraryActionKey.program(id), async () => {
      await api.deleteProgram(id);
      setLibrary((current) => ({ ...current, programs: current.programs.filter((item) => item.id !== id) }));
    });
  }

  async function exportPatternFile(id: string) {
    await withBusy(libraryActionKey.exportPattern(id), async () => exportFile(() => api.exportPattern(id)));
  }

  async function exportProgramFile(id: string) {
    await withBusy(libraryActionKey.exportProgram(id), async () => exportFile(() => api.exportProgram(id)));
  }

  async function playProgram(id: string, intensity: number) {
    await withBusy(libraryActionKey.motionStart, async () => {
      await api.playProgram(id, intensity);
      refresh();
    });
  }

  async function pausePlayer() {
    await withBusy(libraryActionKey.playerControl, async () => {
      await api.pauseMotion();
      refresh();
    });
  }

  async function resumePlayer() {
    await withBusy(libraryActionKey.playerControl, async () => {
      await api.resumeMotion();
      refresh();
    });
  }

  async function stopPlayer() {
    await withBusy(libraryActionKey.playerStop, async () => {
      await api.stopMotion();
      refresh();
    });
  }

  async function setAutoDisable(enabled: boolean) {
    await withBusy(libraryActionKey.autoDisable, async () => {
      const response = await api.setPatternAutoDisable(enabled);
      setLibrary((current) => ({ ...current, auto_disable: response.auto_disable }));
    });
  }

  async function withBusy(key: string, action: () => Promise<unknown>): Promise<boolean> {
    if (inFlightActions.current.has(key)) return false;
    inFlightActions.current.add(key);
    setBusyKeys(new Set(inFlightActions.current));
    try {
      await action();
      return true;
    } catch (error) {
      show(error instanceof Error ? error.message : "Pattern library action failed.", "error");
      return false;
    } finally {
      inFlightActions.current.delete(key);
      if (mounted.current) setBusyKeys(new Set(inFlightActions.current));
    }
  }

  function selectAdjacentTab(event: ReactKeyboardEvent<HTMLButtonElement>, tab: View) {
    let nextIndex: number | undefined;
    const index = views.indexOf(tab);
    if (event.key === "ArrowRight") nextIndex = (index + 1) % views.length;
    if (event.key === "ArrowLeft") nextIndex = (index - 1 + views.length) % views.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = views.length - 1;
    if (nextIndex === undefined) return;
    event.preventDefault();
    const next = views[nextIndex];
    setView(next);
    tabRefs.current[next]?.focus();
  }

  return (
    <>
      <WorkspaceHead title="Pattern library" />
      <section className="panel library-shell" data-requires-backend aria-busy={loading || undefined}>
        <nav className="library-tabs" aria-label="Pattern library views" role="tablist">
          {views.map((tab) => (
            <button
              type="button"
              role="tab"
              key={tab}
              id={`library-${tab}-tab`}
              ref={(node) => { tabRefs.current[tab] = node; }}
              aria-controls={`library-${tab}-panel`}
              aria-selected={view === tab}
              data-active={view === tab || undefined}
              tabIndex={view === tab ? 0 : -1}
              onClick={() => setView(tab)}
              onKeyDown={(event) => selectAdjacentTab(event, tab)}
            >
              {tab === "author" ? "Author" : tab[0].toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </nav>

        {loading ? (
          <div className="empty-state compact-empty" role="status"><h2>Loading library</h2></div>
        ) : loadError ? (
          <div className="empty-state compact-empty" role="alert">
            <h2>Library unavailable</h2>
            <p>{loadError}</p>
            <button type="button" className="btn btn-secondary" onClick={() => void load()}>Retry</button>
          </div>
        ) : (
          <>
            <div role="tabpanel" id="library-browse-panel" aria-labelledby="library-browse-tab" hidden={view !== "browse"}>
              <PatternBrowser patterns={library.patterns} locked={locked} offline={!backendOnline} busyKeys={busyKeys} onPatch={patchPattern} onPlay={playPattern} onFeedback={ratePattern} onExport={exportPatternFile} onDelete={removePattern} />
            </div>
            <div role="tabpanel" id="library-programs-panel" aria-labelledby="library-programs-tab" hidden={view !== "programs"}>
              <ProgramLibrary
                programs={library.programs}
                engine={motion?.engine}
                locked={locked}
                offline={!backendOnline}
                busyKeys={busyKeys}
                maxIntensity={maxIntensity}
                onPlay={playProgram}
                onPause={pausePlayer}
                onResume={resumePlayer}
                onStop={stopPlayer}
                onExport={exportProgramFile}
                onDelete={removeProgram}
              />
            </div>
            <div role="tabpanel" id="library-import-panel" aria-labelledby="library-import-tab" hidden={view !== "import"}>
              <MotionImport locked={locked} importing={busyKeys.has(libraryActionKey.import)} onImport={importFile} />
            </div>
            <div role="tabpanel" id="library-author-panel" aria-labelledby="library-author-tab" hidden={view !== "author"}>
              <PatternAuthoring locked={locked} saving={busyKeys.has(libraryActionKey.author)} onPreview={previewPattern} onPreviewError={showPreviewError} onSave={savePattern} />
            </div>
            <div role="tabpanel" id="library-training-panel" aria-labelledby="library-training-tab" hidden={view !== "training"}>
              <PatternTraining
                patterns={library.patterns}
                feedback={library.feedback}
                autoDisable={library.auto_disable}
                locked={locked}
                busyKeys={busyKeys}
                maxIntensity={maxIntensity}
                onPlay={playPattern}
                onFeedback={ratePattern}
                onUndo={undoFeedback}
                onAutoDisable={setAutoDisable}
              />
            </div>
          </>
        )}
      </section>
    </>
  );
}

function normalizeLibrary(value?: Partial<PatternLibrary>): PatternLibrary {
  return {
    patterns: Array.isArray(value?.patterns) ? value.patterns : [],
    programs: Array.isArray(value?.programs) ? value.programs : [],
    feedback: Array.isArray(value?.feedback) ? value.feedback : [],
    auto_disable: value?.auto_disable === true,
  };
}

async function exportFile(fetchFile: () => Promise<{ blob: Blob; filename: string }>) {
  const { blob, filename } = await fetchFile();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.hidden = true;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}
