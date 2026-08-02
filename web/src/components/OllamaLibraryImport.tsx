import { useState } from "react";
import { api } from "../api/client";
import type { LLMModelImport, ManagedLLMModel, OllamaModelCandidate, OllamaModelScan } from "../api/types";
import { t, translateKnown } from "../i18n";
import { useToast } from "../state/app-state";
import { formatBytes } from "../util/format";
import { HostPathField } from "./HostPathField";

interface OllamaLibraryImportProps {
  path: string;
  suggestedPath?: string;
  managedModels: ManagedLLMModel[];
  locked: boolean;
  onPathChange: (path: string) => void;
  onImportStarted: (job: LLMModelImport) => void;
}

const errorMessage = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");

export function OllamaLibraryImport({
  path,
  suggestedPath = "",
  managedModels,
  locked,
  onPathChange,
  onImportStarted,
}: OllamaLibraryImportProps) {
  const { show } = useToast();
  const [scan, setScan] = useState<OllamaModelScan | null>(null);
  const [scanning, setScanning] = useState(false);
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const scanPath = path.trim() || suggestedPath;

  async function scanLibrary() {
    setScanning(true);
    try {
      const result = await api.scanOllamaModels(scanPath);
      setScan(result);
      onPathChange(result.path);
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      setScanning(false);
    }
  }

  async function importCandidate(candidate: OllamaModelCandidate) {
    setBusy(candidate.id);
    try {
      const response = await api.importOllamaModel(scan?.path || scanPath, candidate.id);
      onImportStarted(response.import);
      show(t("Import started for {name}.", { name: candidate.name }));
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      setBusy("");
    }
  }

  const imported = new Set(managedModels.map((model) => `sha256:${model.sha256}`));
  const normalizedQuery = query.trim().toLowerCase();
  const candidates = scan?.candidates ?? [];
  const visible = normalizedQuery
    ? candidates.filter((candidate) => [candidate.name, candidate.family, candidate.parameter_size, candidate.quantization]
      .some((value) => value?.toLowerCase().includes(normalizedQuery)))
    : candidates;

  return (
    <div className="ollama-import" aria-label={t("Import models from Ollama")}>
      <div className="model-import-path">
        <HostPathField
          label={t("Ollama models path")}
          kind="directory"
          value={path}
          placeholder={suggestedPath || t("Ollama models directory")}
          disabled={locked || scanning}
          onChange={(nextPath) => {
            setScan(null);
            onPathChange(nextPath);
          }}
        />
        <button type="button" className="btn btn-primary" disabled={locked || scanning || !scanPath} onClick={() => void scanLibrary()}>
          {scanning ? t("Scanning...") : t("Scan library")}
        </button>
      </div>

      {scan && candidates.length === 0 && <p className="empty-state model-empty">{t("No Ollama manifests found.")}</p>}
      {candidates.length > 0 && (
        <>
          <label className="field ollama-candidate-filter">
            <span className="label">{t("Filter models")}</span>
            <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} />
          </label>
          <div className="ollama-candidates" aria-label={t("Ollama models available to import")}>
            {visible.map((candidate) => {
              const alreadyImported = Boolean(candidate.imported_model_id) || imported.has(candidate.digest ?? "");
              const metadata = [candidate.parameter_size, candidate.quantization, formatBytes(candidate.size_bytes), candidate.license]
                .filter(Boolean)
                .join(" | ");
              return (
                <div className="ollama-candidate" key={candidate.id}>
                  <div className="model-identity">
                    <strong>{candidate.name}</strong>
                    {metadata && <span>{metadata}</span>}
                  </div>
                  <div className="model-candidate-result">{alreadyImported ? t("Imported") : candidate.reason || t("Ready to copy")}</div>
                  <button
                    type="button"
                    className="btn btn-secondary"
                    disabled={locked || alreadyImported || !candidate.importable || busy === candidate.id}
                    onClick={() => void importCandidate(candidate)}
                  >
                    {busy === candidate.id ? t("Starting...") : t("Import copy")}
                  </button>
                </div>
              );
            })}
            {!visible.length && <p className="empty-state model-empty">{t("No matching models.")}</p>}
          </div>
        </>
      )}
    </div>
  );
}
