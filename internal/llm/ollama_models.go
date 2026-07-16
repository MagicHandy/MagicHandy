package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	maxOllamaManifests    = 2000
	maxOllamaManifestSize = 1024 * 1024
	maxOllamaConfigSize   = 256 * 1024
	maxOllamaLicenseSize  = 256 * 1024

	ollamaModelMediaType     = "application/vnd.ollama.image.model"
	ollamaLicenseMediaType   = "application/vnd.ollama.image.license"
	ollamaAdapterMediaType   = "application/vnd.ollama.image.adapter"
	ollamaProjectorMediaType = "application/vnd.ollama.image.projector"
)

// OllamaCandidate is one filesystem manifest that may be copied into the
// managed llama.cpp model store.
type OllamaCandidate struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Format          string `json:"format,omitempty"`
	Family          string `json:"family,omitempty"`
	ParameterSize   string `json:"parameter_size,omitempty"`
	Quantization    string `json:"quantization,omitempty"`
	SizeBytes       int64  `json:"size_bytes"`
	Digest          string `json:"digest,omitempty"`
	License         string `json:"license,omitempty"`
	Importable      bool   `json:"importable"`
	Reason          string `json:"reason,omitempty"`
	ImportedModelID string `json:"imported_model_id,omitempty"`

	blobPath string
}

// OllamaScan is a bounded snapshot of one Ollama model library.
type OllamaScan struct {
	Path       string            `json:"path"`
	Candidates []OllamaCandidate `json:"candidates"`
}

type ollamaManifest struct {
	SchemaVersion int           `json:"schemaVersion"`
	Config        ollamaLayer   `json:"config"`
	Layers        []ollamaLayer `json:"layers"`
}

type ollamaLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type ollamaModelConfig struct {
	Format        string   `json:"model_format"`
	Family        string   `json:"model_family"`
	Families      []string `json:"model_families"`
	ParameterSize string   `json:"model_type"`
	Quantization  string   `json:"file_type"`
}

// SuggestedOllamaModelsPath returns Ollama's configured or platform-default
// library root without requiring the daemon to be running.
func SuggestedOllamaModelsPath() string {
	if configured := strings.TrimSpace(os.Getenv("OLLAMA_MODELS")); configured != "" {
		return filepath.Clean(configured)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		userPath := filepath.Join(home, ".ollama", "models")
		if runtime.GOOS != "linux" || directoryExists(userPath) {
			return userPath
		}
	}
	if runtime.GOOS == "linux" {
		return filepath.Join("/usr/share/ollama", ".ollama", "models")
	}
	return ""
}

// ScanOllama parses manifests and validates local model blobs without hashing
// multi-gigabyte files. The digest is verified during the explicit copy.
func (m *ModelManager) ScanOllama(ctx context.Context, root string) (OllamaScan, error) {
	resolved, err := resolveOllamaRoot(root)
	if err != nil {
		return OllamaScan{}, err
	}
	imported, err := m.importedDigests(ctx)
	if err != nil {
		return OllamaScan{}, err
	}

	candidates := make([]OllamaCandidate, 0)
	manifestRoot := filepath.Join(resolved, "manifests")
	err = filepath.WalkDir(manifestRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if len(candidates) >= maxOllamaManifests {
			return fmt.Errorf("model library for Ollama exceeds the %d manifest scan limit", maxOllamaManifests)
		}
		relative, err := filepath.Rel(manifestRoot, path)
		if err != nil {
			return err
		}
		candidate := scanOllamaManifest(resolved, path, relative)
		if record, ok := imported[strings.TrimPrefix(candidate.Digest, "sha256:")]; ok {
			candidate.ImportedModelID = record.ID
			if record.State != modelStateReady {
				candidate.Importable = false
				candidate.Reason = fmt.Sprintf(
					"existing managed copy is %s; remove it before importing again",
					record.State,
				)
			}
		}
		candidates = append(candidates, candidate)
		return nil
	})
	if err != nil {
		return OllamaScan{}, fmt.Errorf("scan Ollama manifests: %w", err)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := strings.ToLower(candidates[i].Name), strings.ToLower(candidates[j].Name)
		if left == right {
			return candidates[i].ID < candidates[j].ID
		}
		return left < right
	})
	return OllamaScan{Path: resolved, Candidates: candidates}, nil
}

func scanOllamaManifest(root, path, relative string) OllamaCandidate {
	candidate := OllamaCandidate{Name: ollamaNameFromManifest(relative)}
	payload, err := readLimitedFile(path, maxOllamaManifestSize)
	if err != nil {
		return rejectOllamaCandidate(candidate, relative, "manifest is unreadable")
	}
	var manifest ollamaManifest
	if err := json.Unmarshal(payload, &manifest); err != nil || manifest.SchemaVersion != 2 {
		return rejectOllamaCandidate(candidate, relative, "manifest is invalid or unsupported")
	}

	models := layersByMediaType(manifest.Layers, ollamaModelMediaType)
	candidate.ID = ollamaCandidateID(relative, firstLayerDigest(models))
	if len(models) != 1 {
		candidate.Reason = "requires exactly one GGUF model layer"
		return candidate
	}
	if hasLayer(manifest.Layers, ollamaAdapterMediaType) || hasLayer(manifest.Layers, ollamaProjectorMediaType) {
		candidate.Reason = "requires adapter or projector layers that managed llama.cpp does not load"
		return candidate
	}

	modelLayer := models[0]
	candidate.Digest = normalizeSHA256Digest(modelLayer.Digest)
	candidate.SizeBytes = modelLayer.Size
	if candidate.Digest == "" || candidate.SizeBytes <= 0 {
		candidate.Reason = "model layer digest or size is invalid"
		return candidate
	}
	if candidate.SizeBytes > maxManagedModelBytes {
		candidate.Reason = fmt.Sprintf("model layer exceeds the %d-byte import limit", maxManagedModelBytes)
		return candidate
	}
	candidate.blobPath = ollamaBlobPath(root, candidate.Digest)
	if reason := validateOllamaBlob(candidate.blobPath, candidate.SizeBytes); reason != "" {
		candidate.Reason = reason
		return candidate
	}

	config := readOllamaConfig(root, manifest.Config)
	candidate.Format = strings.ToLower(strings.TrimSpace(config.Format))
	if candidate.Format == "" {
		candidate.Format = "gguf"
	}
	candidate.Family = firstNonEmpty(config.Family, firstString(config.Families))
	candidate.ParameterSize = strings.TrimSpace(config.ParameterSize)
	candidate.Quantization = strings.TrimSpace(config.Quantization)
	candidate.License = readOllamaLicense(root, manifest.Layers)
	if candidate.Format != "gguf" {
		candidate.Reason = fmt.Sprintf("model format %q is not GGUF", candidate.Format)
		return candidate
	}
	candidate.Importable = true
	return candidate
}

func rejectOllamaCandidate(candidate OllamaCandidate, relative, reason string) OllamaCandidate {
	candidate.ID = ollamaCandidateID(relative, "")
	candidate.Reason = reason
	return candidate
}

func validateOllamaBlob(path string, expectedSize int64) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "model blob is missing"
	}
	if info.Size() != expectedSize {
		return "model blob size does not match its manifest"
	}
	file, err := os.Open(path) // #nosec G304 -- bounded reads are limited to a user-selected Ollama library.
	if err != nil {
		return "model blob is unreadable"
	}
	defer func() { _ = file.Close() }()
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil || string(magic[:]) != "GGUF" {
		return "model layer is not a GGUF file"
	}
	return ""
}

func readOllamaConfig(root string, layer ollamaLayer) ollamaModelConfig {
	digest := normalizeSHA256Digest(layer.Digest)
	if digest == "" || layer.Size <= 0 || layer.Size > maxOllamaConfigSize {
		return ollamaModelConfig{}
	}
	payload, err := readLimitedFile(ollamaBlobPath(root, digest), maxOllamaConfigSize)
	if err != nil {
		return ollamaModelConfig{}
	}
	var config ollamaModelConfig
	_ = json.Unmarshal(payload, &config)
	return config
}

func readOllamaLicense(root string, layers []ollamaLayer) string {
	for _, layer := range layers {
		if layer.MediaType != ollamaLicenseMediaType || layer.Size <= 0 || layer.Size > maxOllamaLicenseSize {
			continue
		}
		digest := normalizeSHA256Digest(layer.Digest)
		if digest == "" {
			continue
		}
		payload, err := readLimitedFile(ollamaBlobPath(root, digest), maxOllamaLicenseSize)
		if err == nil {
			return firstLicenseLine(string(payload))
		}
	}
	return ""
}

func (m *ModelManager) importedDigests(ctx context.Context) (map[string]ModelRecord, error) {
	models, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]ModelRecord, len(models))
	for _, model := range models {
		result[model.SHA256] = model
	}
	return result, nil
}

func resolveOllamaRoot(value string) (string, error) {
	value = expandHome(strings.TrimSpace(value))
	if value == "" {
		value = SuggestedOllamaModelsPath()
	}
	if value == "" {
		return "", errors.New("model library path for Ollama is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve Ollama model library: %w", err)
	}
	candidates := []string{absolute, filepath.Join(absolute, "models")}
	if strings.EqualFold(filepath.Base(absolute), "manifests") {
		candidates = append([]string{filepath.Dir(absolute)}, candidates...)
	}
	for _, candidate := range candidates {
		if directoryExists(filepath.Join(candidate, "manifests")) && directoryExists(filepath.Join(candidate, "blobs")) {
			resolved, evalErr := filepath.EvalSymlinks(candidate)
			if evalErr != nil {
				return "", fmt.Errorf("resolve Ollama model library links: %w", evalErr)
			}
			return resolved, nil
		}
	}
	return "", fmt.Errorf("model library for Ollama %q must contain manifests and blobs directories", absolute)
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path) // #nosec G304 -- this helper only performs caller-bounded reads of explicit local model metadata.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("file exceeds read limit")
	}
	return payload, nil
}

func layersByMediaType(layers []ollamaLayer, mediaType string) []ollamaLayer {
	result := make([]ollamaLayer, 0, 1)
	for _, layer := range layers {
		if layer.MediaType == mediaType {
			result = append(result, layer)
		}
	}
	return result
}

func hasLayer(layers []ollamaLayer, mediaType string) bool {
	return len(layersByMediaType(layers, mediaType)) > 0
}

func firstLayerDigest(layers []ollamaLayer) string {
	if len(layers) == 0 {
		return ""
	}
	return layers[0].Digest
}

func normalizeSHA256Digest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return ""
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return ""
	}
	return value
}

func ollamaBlobPath(root, digest string) string {
	return filepath.Join(root, "blobs", strings.Replace(digest, ":", "-", 1))
}

func ollamaCandidateID(relative, digest string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(relative) + "\x00" + strings.ToLower(digest)))
	return "ollama-" + hex.EncodeToString(sum[:12])
}

func ollamaNameFromManifest(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 3 {
		return filepath.ToSlash(relative)
	}
	registry := parts[0]
	repository := parts[1 : len(parts)-1]
	tag := parts[len(parts)-1]
	if len(repository) > 1 && repository[0] == "library" {
		repository = repository[1:]
	}
	name := strings.Join(repository, "/")
	if registry != "registry.ollama.ai" {
		name = registry + "/" + name
	}
	return name + ":" + tag
}

func firstLicenseLine(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 160 {
			line = string(runes[:160])
		}
		return line
	}
	return ""
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func expandHome(value string) string {
	if value != "~" && !strings.HasPrefix(value, "~"+string(filepath.Separator)) && !strings.HasPrefix(value, "~/") {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return value
	}
	if value == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimLeft(value[1:], "/\\"))
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
