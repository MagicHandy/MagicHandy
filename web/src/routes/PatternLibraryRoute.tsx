import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { LibraryPattern, PatternInput, PatternLibrary, PatternPreview } from "../api/types";
import { PatternAuthoring } from "../components/PatternAuthoring";
import { PatternBrowser } from "../components/PatternBrowser";
import { PatternTraining } from "../components/PatternTraining";
import { ProgramLibrary } from "../components/ProgramLibrary";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

type View = "browse" | "programs" | "author" | "training";
const emptyLibrary: PatternLibrary = { patterns: [], programs: [], feedback: [], auto_disable: false };

export function PatternLibraryRoute() {
  const { backendOnline, readOnly, motion, state, refresh } = useAppState();
  const { show } = useToast();
  const [view, setView] = useState<View>("browse");
  const [library, setLibrary] = useState<PatternLibrary>(emptyLibrary);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState("");
  const locked = !backendOnline || readOnly;
  const maxIntensity = state?.settings?.motion?.speed_max_percent ?? 100;

  async function load() {
    try {
      const response = await api.getLibrary();
      setLibrary(normalizeLibrary(response?.library));
    } catch (error) {
      show(error instanceof Error ? error.message : "Pattern library could not be loaded.", "error");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  function replacePattern(pattern: LibraryPattern) {
    setLibrary((current) => ({ ...current, patterns: current.patterns.map((item) => item.id === pattern.id ? pattern : item) }));
  }

  async function patchPattern(id: string, patch: Partial<Pick<LibraryPattern, "enabled" | "weight">>) {
    await withBusy(id, async () => {
      const response = await api.patchPattern(id, patch);
      replacePattern(response.pattern);
    });
  }

  async function playPattern(id: string, intensity = Math.min(30, maxIntensity), feel = "original") {
    await withBusy(id, async () => {
      await api.playPattern(id, intensity, feel);
      refresh();
    });
  }

  async function ratePattern(id: string, rating: -1 | 1) {
    await withBusy(id, async () => {
      const response = await api.patternFeedback(id, rating);
      replacePattern(response.pattern);
      setLibrary((current) => ({ ...current, feedback: [response.feedback, ...current.feedback] }));
    });
  }

  async function undoFeedback(id: number) {
    await withBusy(`feedback-${id}`, async () => {
      const response = await api.undoPatternFeedback(id);
      replacePattern(response.pattern);
      setLibrary((current) => ({ ...current, feedback: current.feedback.map((item) => item.id === id ? response.feedback : item) }));
    });
  }

  async function savePattern(input: PatternInput) {
    await withBusy("author", async () => {
      const response = await api.createPattern(input);
      setLibrary((current) => ({ ...current, patterns: [...current.patterns, response.pattern] }));
      setView("browse");
      show(`${response.pattern.name} saved.`);
    });
  }

  async function previewPattern(input: PatternInput): Promise<PatternPreview> {
    try {
      return (await api.previewPattern(input)).preview;
    } catch (error) {
      show(error instanceof Error ? error.message : "Pattern preview failed.", "error");
      throw error;
    }
  }

  async function importFile(file: File, asKind: "pattern" | "program") {
    await withBusy("import", async () => {
      await api.importMotionContent(file, asKind);
      await load();
      show(`${file.name} imported.`);
    });
  }

  async function removePattern(id: string) {
    const pattern = library.patterns.find((item) => item.id === id);
    if (!pattern || !window.confirm(`Delete ${pattern.name}?`)) return;
    await withBusy(id, async () => {
      await api.deletePattern(id);
      setLibrary((current) => ({ ...current, patterns: current.patterns.filter((item) => item.id !== id) }));
    });
  }

  async function removeProgram(id: string) {
    const program = library.programs.find((item) => item.id === id);
    if (!program || !window.confirm(`Delete ${program.name}?`)) return;
    await withBusy(id, async () => {
      await api.deleteProgram(id);
      setLibrary((current) => ({ ...current, programs: current.programs.filter((item) => item.id !== id) }));
    });
  }

  async function exportPatternFile(id: string) {
    await withBusy(`export-${id}`, async () => exportFile(() => api.exportPattern(id)));
  }

  async function exportProgramFile(id: string) {
    await withBusy(`export-${id}`, async () => exportFile(() => api.exportProgram(id)));
  }

  async function withBusy(id: string, action: () => Promise<void>) {
    setBusyId(id);
    try {
      await action();
    } catch (error) {
      show(error instanceof Error ? error.message : "Pattern library action failed.", "error");
    } finally {
      setBusyId("");
    }
  }

  return (
    <>
      <WorkspaceHead title="Pattern library" />
      <section className="panel library-shell" data-requires-backend>
        <nav className="library-tabs" aria-label="Pattern library views" role="tablist">
          {(["browse", "programs", "author", "training"] as const).map((tab) => <button type="button" role="tab" key={tab} id={`library-${tab}-tab`} aria-controls={`library-${tab}-panel`} aria-selected={view === tab} data-active={view === tab || undefined} onClick={() => setView(tab)}>{tab === "author" ? "Author" : tab[0].toUpperCase() + tab.slice(1)}</button>)}
        </nav>
        <div role="tabpanel" id={`library-${view}-panel`} aria-labelledby={`library-${view}-tab`}>
          {loading ? <div className="empty-state compact-empty"><h2>Loading library</h2></div> : view === "browse" ? (
            <PatternBrowser patterns={library.patterns} locked={locked} offline={!backendOnline} busyId={busyId} onPatch={patchPattern} onPlay={playPattern} onFeedback={ratePattern} onExport={exportPatternFile} onDelete={removePattern} />
          ) : view === "programs" ? (
            <ProgramLibrary programs={library.programs} engine={motion?.engine} locked={locked} offline={!backendOnline} busyId={busyId} maxIntensity={maxIntensity} onImport={importFile} onPlay={(id, intensity) => withBusy(id, async () => { await api.playProgram(id, intensity); refresh(); })} onPause={() => withBusy("player", async () => { await api.pauseMotion(); refresh(); })} onResume={() => withBusy("player", async () => { await api.resumeMotion(); refresh(); })} onStop={() => withBusy("player", async () => { await api.stopMotion(); refresh(); })} onExport={exportProgramFile} onDelete={removeProgram} />
          ) : view === "author" ? (
            <PatternAuthoring locked={locked} saving={busyId === "author"} onPreview={previewPattern} onSave={savePattern} />
          ) : (
            <PatternTraining patterns={library.patterns} feedback={library.feedback} autoDisable={library.auto_disable} locked={locked} busyId={busyId} maxIntensity={maxIntensity} onPlay={playPattern} onFeedback={ratePattern} onUndo={undoFeedback} onAutoDisable={(enabled) => withBusy("auto-disable", async () => { await api.setPatternAutoDisable(enabled); setLibrary((current) => ({ ...current, auto_disable: enabled })); })} />
          )}
        </div>
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
  link.click();
  URL.revokeObjectURL(url);
}
