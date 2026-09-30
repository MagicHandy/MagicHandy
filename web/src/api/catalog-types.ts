export type LLMCatalogFit = "recommended" | "supported" | "below_minimum" | "unknown_vram" | "no_gpu";

// A curated, checksum-pinned model the backend can download. The backend
// rates each entry against the detected GPU; the UI only presents that fit.
export interface LLMCatalogModel {
  id: string;
  display_name: string;
  summary: string;
  family: string;
  parameter_size: string;
  quantization: string;
  size_bytes: number;
  sha256: string;
  license: string;
  license_url: string;
  source_name: string;
  source_url: string;
  download_url: string;
  vram_mib: number;
  min_vram_mib: number;
  default: boolean;
  fit: LLMCatalogFit;
  installed_model_id?: string;
  partial_bytes?: number;
}

export interface LLMCatalog {
  models: LLMCatalogModel[];
  hardware: { nvidia: boolean; gpu_name?: string; vram_mib?: number };
}
