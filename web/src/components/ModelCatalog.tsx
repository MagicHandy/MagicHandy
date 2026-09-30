import { t, translateKnown } from "../i18n";
import type { LLMCatalog, LLMCatalogModel } from "../api/catalog-types";
import type { LLMModelImport } from "../api/types";
import { formatBytes } from "../util/format";

const MIB = 1024 * 1024;

// The backend rates each curated model against the detected GPU; this only
// turns that rating into a sentence the user can act on.
export function catalogFitNote(model: LLMCatalogModel, hardware: LLMCatalog["hardware"]): string {
  const memory = formatBytes(model.vram_mib * MIB);
  const total = formatBytes((hardware.vram_mib ?? 0) * MIB);
  const gpu = hardware.gpu_name || "NVIDIA GPU";
  switch (model.fit) {
    case "recommended":
    case "supported":
      return t("Uses about {memory} of the {total} on your {gpu}.", { memory, total, gpu });
    case "below_minimum":
      return t("Needs about {memory} of graphics memory, but your {gpu} has {total}. Part of the model would run on the CPU and replies would be slow.", { memory, total, gpu });
    case "unknown_vram":
      return t("Needs about {memory} of graphics memory. MagicHandy could not read how much your card has.", { memory });
    default:
      return t("Local chat needs an NVIDIA graphics card. On the CPU alone, replies are too slow to use.");
  }
}

export function catalogHardwareLine(catalog: LLMCatalog | null, nvidia: boolean, gpuName?: string): string {
  if (!nvidia) return t("No NVIDIA graphics card was found. Local chat needs one; on the CPU alone, replies are too slow to use.");
  const vram = catalog?.hardware.vram_mib;
  if (vram) return t("Detected {gpu} with {memory} of graphics memory.", { gpu: gpuName || "NVIDIA GPU", memory: formatBytes(vram * MIB) });
  return t("Detected {gpu}", { gpu: gpuName || "NVIDIA GPU" });
}

export function catalogModelDetail(model: LLMCatalogModel, catalog: LLMCatalog): string {
  if (model.installed_model_id) return t("{summary} Already in your model store.", { summary: translateKnown(model.summary) });
  const partial = model.partial_bytes ?? 0;
  const size = partial > 0
    ? t("{saved} of {total} already downloaded; the rest resumes.", { saved: formatBytes(partial), total: formatBytes(model.size_bytes) })
    : t("{size} download.", { size: formatBytes(model.size_bytes) });
  return `${translateKnown(model.summary)} ${size} ${catalogFitNote(model, catalog.hardware)}`;
}

export function CatalogSourceLine({ model }: { model: LLMCatalogModel }) {
  return <p className="hint-block model-catalog-source">
    <span>{t("License: {license}", { license: model.license })}</span>
    <a href={model.license_url} target="_blank" rel="noreferrer">{t("License terms")}</a>
    <a href={model.source_url} target="_blank" rel="noreferrer">{t("Model page")}</a>
    <span>{t("Verified by SHA-256 before it is added.")}</span>
  </p>;
}

// Settings view: the same curated entries with explicit per-model downloads.
export function ModelCatalogDownloads({ catalog, imports, locked, busy, onDownload }: {
  catalog: LLMCatalog | null;
  imports: LLMModelImport[];
  locked: boolean;
  busy: string;
  onDownload: (model: LLMCatalogModel) => void;
}) {
  if (!catalog?.models.length) return null;
  const active = new Set(imports.filter((job) => job.status === "queued" || job.status === "downloading").map((job) => job.display_name));
  return <section className="model-catalog" aria-labelledby="model-catalog-title">
    <div className="model-catalog-head">
      <h5 id="model-catalog-title" className="group-title">{t("Download a tested model")}</h5>
      <p className="form-status">{t("Each model is checked against its pinned SHA-256 before it enters the store.")}</p>
    </div>
    <div className="model-list">
      {catalog.models.map((model) => {
        const downloading = active.has(model.display_name);
        return <div className="model-row model-catalog-row" key={model.id}>
          <div className="model-identity">
            <strong>{model.display_name}{model.fit === "recommended" && <span className="setup-badge model-catalog-badge">{t("Recommended")}</span>}</strong>
            <span>{catalogModelDetail(model, catalog)}</span>
            <CatalogSourceLine model={model} />
          </div>
          <div className="model-row-actions">
            {model.installed_model_id
              ? <span className="model-state model-state-ready">{t("In your store")}</span>
              : <button type="button" className="btn btn-secondary" disabled={locked || downloading || busy === `catalog:${model.id}`} onClick={() => onDownload(model)}>
                {downloading ? t("Downloading...") : (model.partial_bytes ?? 0) > 0 ? t("Resume download") : t("Download")}
              </button>}
          </div>
        </div>;
      })}
    </div>
  </section>;
}
