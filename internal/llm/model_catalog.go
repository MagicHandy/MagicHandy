package llm

import (
	"strings"
)

// Catalog fit values describe how one curated model matches the detected GPU.
const (
	// CatalogFitRecommended marks the first curated model, in catalog order,
	// that the detected GPU holds.
	CatalogFitRecommended = "recommended"
	// CatalogFitSupported marks another curated model the hardware holds.
	CatalogFitSupported = "supported"
	// CatalogFitBelowMinimum means the GPU has less memory than the model needs.
	CatalogFitBelowMinimum = "below_minimum"
	// CatalogFitUnknownVRAM means an NVIDIA GPU was found but its memory was not.
	CatalogFitUnknownVRAM = "unknown_vram"
	// CatalogFitNoGPU means no NVIDIA GPU was found; local chat would be too slow.
	CatalogFitNoGPU = "no_gpu"
)

// CatalogModel is one curated, checksum-pinned model the app can download.
//
// Entries are content-addressed Ollama registry blobs, whose URL names the
// SHA-256 digest, or Hugging Face files pinned to one repository commit. Either
// way the download is verified against SHA256 before it enters the store. The
// catalog ships with each release; nothing fetches or updates it at runtime.
type CatalogModel struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Summary       string `json:"summary"`
	Family        string `json:"family"`
	ParameterSize string `json:"parameter_size"`
	Quantization  string `json:"quantization"`
	SizeBytes     int64  `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	License       string `json:"license"`
	LicenseURL    string `json:"license_url"`
	SourceName    string `json:"source_name"`
	SourceURL     string `json:"source_url"`
	DownloadURL   string `json:"download_url"`
	// VRAMMiB is the dedicated GPU memory the model uses at the default context.
	VRAMMiB int `json:"vram_mib"`
	// MinVRAMMiB leaves room for the desktop beside VRAMMiB.
	MinVRAMMiB int `json:"min_vram_mib"`
	// Default marks the model the prompts are tuned against; setup chooses it
	// when the GPU's memory cannot be read.
	Default bool `json:"default"`
}

// CatalogHardware carries the GPU facts a recommendation needs.
type CatalogHardware struct {
	NVIDIA  bool
	VRAMMiB int
}

const (
	ollamaRegistryBlobBase = "https://registry.ollama.ai/v2/"
	huggingFaceBase        = "https://huggingface.co/"
)

// The curated catalog, in preference order: the first entry a GPU holds is
// recommended for it (docs/model-suitability.md has every measurement). VRAM
// figures are dedicated GPU memory at the default 32,768-token context on the
// managed CUDA runtime.
//
//   - The 12B heretic default: the prompts were tuned against it, and it
//     measured the most explicit replies of the 12B builds.
//   - A 12B QAT build: the same contract results in about 0.6 GB less memory.
//   - An E4B QAT build for 6 to 8 GB cards. Its embedded chat template needs
//     the Gemma 4 thinking fix, which the runner applies by itself
//     (knownTemplateFixes); without it every reply reasons silently first.
var curatedCatalog = []CatalogModel{
	{
		ID:            "gemma-4-12b-heretic-q4km",
		DisplayName:   "Gemma 4 12B Heretic (Q4_K_M)",
		Summary:       "The tested default: reliable motion commands and replies in about two seconds.",
		Family:        "gemma4",
		ParameterSize: "11.9B",
		Quantization:  "Q4_K_M",
		SizeBytes:     7381381696,
		SHA256:        "239ec362963977a0dc5de2402c71de39c1d96b2f6ef73d944c01f47281a038c2",
		License:       "Apache-2.0 (Gemma 4)",
		LicenseURL:    "https://ai.google.dev/gemma/docs/gemma_4_license",
		SourceName:    "n0404n0404/gemma-4-12b-it-heretic-b9a462:latest",
		SourceURL:     "https://ollama.com/n0404n0404/gemma-4-12b-it-heretic-b9a462",
		DownloadURL: ollamaRegistryBlobBase + "n0404n0404/gemma-4-12b-it-heretic-b9a462/blobs/" +
			"sha256:239ec362963977a0dc5de2402c71de39c1d96b2f6ef73d944c01f47281a038c2",
		VRAMMiB:    8400,
		MinVRAMMiB: 10240,
		Default:    true,
	},
	{
		ID:            "gemma-4-12b-qat-heretic-v3-q4-0",
		DisplayName:   "Gemma 4 12B QAT Heretic v3 (Q4_0)",
		Summary:       "A lighter 12B: the same reliable motion commands in less graphics memory.",
		Family:        "gemma4",
		ParameterSize: "11.9B",
		Quantization:  "Q4_0",
		SizeBytes:     6716356480,
		SHA256:        "5f2b07f447116d103eaa9b9a2b2843f6c0c3f35da487f5b8dc40313aa3697778",
		License:       "Apache-2.0 (Gemma 4)",
		LicenseURL:    "https://ai.google.dev/gemma/docs/gemma_4_license",
		SourceName:    "OS-Software/gemma-4-12B-it-qat-q4_0-uncensored-heretic-v3-GGUF",
		SourceURL:     huggingFaceBase + "OS-Software/gemma-4-12B-it-qat-q4_0-uncensored-heretic-v3-GGUF",
		DownloadURL: huggingFaceBase + "OS-Software/gemma-4-12B-it-qat-q4_0-uncensored-heretic-v3-GGUF/resolve/" +
			"7e1f9bc1a2056f68a3307d88e76e87ee7588bee6/gemma-4-12B-it-qat-q4_0-uncensored-heretic-v3-Q4_0.gguf",
		VRAMMiB:    7766,
		MinVRAMMiB: 10240,
	},
	{
		ID:            "gemma-4-e4b-qat-heretic-q4-0",
		DisplayName:   "Gemma 4 E4B QAT Heretic (Q4_0)",
		Summary:       "For 6 to 8 GB graphics cards: the fastest tested model, with shorter replies and simpler motion.",
		Family:        "gemma4",
		ParameterSize: "7.5B",
		Quantization:  "Q4_0",
		SizeBytes:     4255261568,
		SHA256:        "063b60c663f52221172f1b7c1a3d929c423134bef16b3f61fae3c64bc7ff0c30",
		License:       "Apache-2.0 (Gemma 4)",
		LicenseURL:    "https://ai.google.dev/gemma/docs/gemma_4_license",
		SourceName:    "HTNZ555/gemma-4-E4B-it-heretic-QAT-GGUF",
		SourceURL:     huggingFaceBase + "HTNZ555/gemma-4-E4B-it-heretic-QAT-GGUF",
		DownloadURL: huggingFaceBase + "HTNZ555/gemma-4-E4B-it-heretic-QAT-GGUF/resolve/" +
			"5b3ddc78cce49de4ad31e827b851797eb2498ad9/gemma-4-E4B-it-heretic-QAT-UD-Q4_K_XL.gguf",
		VRAMMiB:    3446,
		MinVRAMMiB: 6144,
	},
}

// CatalogModels returns a copy of the curated download catalog.
func CatalogModels() []CatalogModel {
	return append([]CatalogModel(nil), curatedCatalog...)
}

// FindCatalogModel returns one curated entry by its stable ID.
func FindCatalogModel(id string) (CatalogModel, bool) {
	id = strings.TrimSpace(id)
	for _, model := range curatedCatalog {
		if model.ID == id {
			return model, true
		}
	}
	return CatalogModel{}, false
}

// CatalogFit rates one curated model against the detected GPU. Only measured
// memory needs are used: a smaller card is told the model will not fit rather
// than being offered an untested recommendation. The first entry in catalog
// order that fits is recommended, so each card gets the best model it holds.
func CatalogFit(model CatalogModel, hardware CatalogHardware) string {
	switch {
	case !hardware.NVIDIA:
		return CatalogFitNoGPU
	case hardware.VRAMMiB <= 0:
		return CatalogFitUnknownVRAM
	case hardware.VRAMMiB < model.MinVRAMMiB:
		return CatalogFitBelowMinimum
	case model.ID == recommendedCatalogID(hardware):
		return CatalogFitRecommended
	default:
		return CatalogFitSupported
	}
}

func recommendedCatalogID(hardware CatalogHardware) string {
	for _, model := range curatedCatalog {
		if hardware.VRAMMiB >= model.MinVRAMMiB {
			return model.ID
		}
	}
	return ""
}

// catalogSource records how a curated download is labelled in the store.
func catalogSource(model CatalogModel) string {
	if strings.HasPrefix(model.DownloadURL, ollamaRegistryBlobBase) {
		return ModelSourceOllama
	}
	return ModelSourceGGUF
}

func catalogPartialName(sha string) string {
	return "catalog-" + strings.ToLower(sha) + ".partial"
}

func catalogSHAKnown(sha string) bool {
	for _, model := range curatedCatalog {
		if strings.EqualFold(model.SHA256, sha) {
			return true
		}
	}
	return false
}
