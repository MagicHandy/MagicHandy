package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// ImportStatusQueued identifies an import waiting for its copy goroutine.
	ImportStatusQueued = "queued"
	// ImportStatusCopying identifies an active verified copy.
	ImportStatusCopying = "copying"
	// ImportStatusComplete identifies a committed managed model.
	ImportStatusComplete = "complete"
	// ImportStatusFailed identifies an import that did not commit.
	ImportStatusFailed = "failed"
	// ImportStatusCancelled identifies an explicitly cancelled import.
	ImportStatusCancelled = "cancelled"

	maxManagedModelBytes = int64(1 << 40) // 1 TiB safety bound, not a recommendation.
	maxTrackedImportJobs = 64
	maxConcurrentImports = 2
	maxModelNameChars    = 120
)

// ImportJob is a progress snapshot for one explicit model copy.
type ImportJob struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	BytesCopied int64  `json:"bytes_copied"`
	TotalBytes  int64  `json:"total_bytes"`
	ModelID     string `json:"model_id,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"started_at"`
	UpdatedAt   string `json:"updated_at"`
}

type modelImportJob struct {
	snapshot  ImportJob
	cancel    context.CancelFunc
	sourceKey string
}

type modelImportSpec struct {
	DisplayName   string
	Source        string
	SourceName    string
	SourcePath    string
	ExpectedSHA   string
	SizeBytes     int64
	Format        string
	Family        string
	ParameterSize string
	Quantization  string
	License       string
}

// StartOllamaImport revalidates a scanned candidate and starts an atomic copy.
func (m *ModelManager) StartOllamaImport(ctx context.Context, root, candidateID string) (ImportJob, error) {
	scan, err := m.ScanOllama(ctx, root)
	if err != nil {
		return ImportJob{}, err
	}
	var selected *OllamaCandidate
	for index := range scan.Candidates {
		if scan.Candidates[index].ID == strings.TrimSpace(candidateID) {
			selected = &scan.Candidates[index]
			break
		}
	}
	if selected == nil {
		return ImportJob{}, errors.New("selected Ollama model changed or is no longer present; scan again")
	}
	if !selected.Importable {
		return ImportJob{}, fmt.Errorf("selected Ollama model cannot be imported: %s", selected.Reason)
	}
	if selected.ImportedModelID != "" {
		return m.completedImport(*selected)
	}
	return m.startImport(modelImportSpec{
		DisplayName: selected.Name, Source: ModelSourceOllama,
		SourceName: selected.Name, SourcePath: selected.blobPath,
		ExpectedSHA: strings.TrimPrefix(selected.Digest, "sha256:"), SizeBytes: selected.SizeBytes,
		Format: selected.Format, Family: selected.Family, ParameterSize: selected.ParameterSize,
		Quantization: selected.Quantization, License: selected.License,
	})
}

// StartGGUFImport starts a managed copy of one user-selected GGUF file.
func (m *ModelManager) StartGGUFImport(path, displayName string) (ImportJob, error) {
	absolute, err := filepath.Abs(expandHome(strings.TrimSpace(path)))
	if err != nil {
		return ImportJob{}, fmt.Errorf("resolve GGUF model path: %w", err)
	}
	if pathWithin(m.modelsDir, absolute) {
		return ImportJob{}, errors.New("GGUF model is already inside the managed model store")
	}
	info, err := validateGGUFFile(absolute)
	if err != nil {
		return ImportJob{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	}
	return m.startImport(modelImportSpec{
		DisplayName: displayName,
		Source:      ModelSourceGGUF,
		SourceName:  filepath.Base(absolute),
		SourcePath:  absolute,
		SizeBytes:   info.Size(),
		Format:      "gguf",
	})
}

// Import returns one import progress snapshot.
func (m *ModelManager) Import(id string) (ImportJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[strings.TrimSpace(id)]
	if !ok {
		return ImportJob{}, ErrImportNotFound
	}
	return job.snapshot, nil
}

// ImportJobs returns newest jobs first.
func (m *ModelManager) ImportJobs() []ImportJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]ImportJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job.snapshot)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].StartedAt > jobs[j].StartedAt })
	return jobs
}

// CancelImport cancels a queued or active copy. Completed history remains.
func (m *ModelManager) CancelImport(id string) (ImportJob, error) {
	m.mu.Lock()
	job, ok := m.jobs[strings.TrimSpace(id)]
	if !ok {
		m.mu.Unlock()
		return ImportJob{}, ErrImportNotFound
	}
	if job.cancel != nil {
		job.cancel()
	}
	snapshot := job.snapshot
	m.mu.Unlock()
	return snapshot, nil
}

func (m *ModelManager) startImport(spec modelImportSpec) (ImportJob, error) {
	spec = normalizeImportSpec(spec)
	if err := validateImportSpec(spec); err != nil {
		return ImportJob{}, err
	}
	sourceKey := spec.Source + "\x00" + spec.SourcePath + "\x00" + spec.ExpectedSHA

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ImportJob{}, errors.New("model manager is closed")
	}
	m.pruneImportJobsLocked()
	activeCount := 0
	for _, existing := range m.jobs {
		if existing.sourceKey == sourceKey && activeImportStatus(existing.snapshot.Status) {
			return existing.snapshot, nil
		}
		if activeImportStatus(existing.snapshot.Status) {
			activeCount++
		}
	}
	if activeCount >= maxConcurrentImports {
		return ImportJob{}, fmt.Errorf("at most %d model imports can run at once", maxConcurrentImports)
	}
	id, err := randomImportID()
	if err != nil {
		return ImportJob{}, err
	}
	now := nowText()
	ctx, cancel := context.WithCancel(context.Background())
	job := &modelImportJob{
		snapshot: ImportJob{
			ID: id, Source: spec.Source, DisplayName: spec.DisplayName,
			Status: ImportStatusQueued, TotalBytes: spec.SizeBytes,
			StartedAt: now, UpdatedAt: now,
		},
		cancel: cancel, sourceKey: sourceKey,
	}
	m.jobs[id] = job
	m.wg.Add(1)
	go m.runImport(ctx, id, spec)
	return job.snapshot, nil
}

func (m *ModelManager) runImport(ctx context.Context, jobID string, spec modelImportSpec) {
	defer m.wg.Done()
	temporary := filepath.Join(m.downloadsDir, "model-import-"+jobID+".partial")
	digest, copied, err := m.copyModel(ctx, jobID, spec, temporary)
	if err != nil {
		m.finishImport(jobID, "", cleanupImportPartial(temporary, err))
		return
	}
	if _, err := validateGGUFFile(temporary); err != nil {
		m.finishImport(jobID, "", cleanupImportPartial(temporary, fmt.Errorf("validate copied model: %w", err)))
		return
	}
	record, err := m.commitModel(ctx, spec, temporary, digest, copied)
	if err != nil {
		m.finishImport(jobID, "", cleanupImportPartial(temporary, err))
		return
	}
	m.finishImport(jobID, record.ID, nil)
}

func (m *ModelManager) copyModel(
	ctx context.Context,
	jobID string,
	spec modelImportSpec,
	temporary string,
) (string, int64, error) {
	source, err := os.Open(spec.SourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("open model source: %w", err)
	}
	defer func() { _ = source.Close() }()
	// #nosec G304 -- temporary is constructed under the private app downloads directory.
	destination, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create model import: %w", err)
	}

	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var copied int64
	copyErr := error(nil)
	m.updateImportProgress(jobID, ImportStatusCopying, 0)
	limitedSource := io.LimitReader(source, spec.SizeBytes+1)
	for {
		if err := ctx.Err(); err != nil {
			copyErr = err
			break
		}
		count, readErr := limitedSource.Read(buffer)
		if count > 0 {
			if _, err := destination.Write(buffer[:count]); err != nil {
				copyErr = err
				break
			}
			_, _ = hash.Write(buffer[:count])
			copied += int64(count)
			m.updateImportProgress(jobID, ImportStatusCopying, copied)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			copyErr = readErr
			break
		}
	}
	if copyErr == nil {
		if syncErr := destination.Sync(); syncErr != nil {
			copyErr = syncErr
		}
	}
	if closeErr := destination.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return "", copied, fmt.Errorf("copy model: %w", copyErr)
	}
	if copied != spec.SizeBytes {
		return "", copied, fmt.Errorf("model size changed during import: copied %d of %d bytes", copied, spec.SizeBytes)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if spec.ExpectedSHA != "" && !strings.EqualFold(digest, spec.ExpectedSHA) {
		return "", copied, errors.New("model SHA-256 does not match the Ollama manifest")
	}
	return digest, copied, nil
}

func (m *ModelManager) commitModel(
	ctx context.Context,
	spec modelImportSpec,
	temporary, digest string,
	size int64,
) (ModelRecord, error) {
	m.inventoryMu.Lock()
	defer m.inventoryMu.Unlock()

	if existing, ok, err := m.modelBySHA(ctx, digest); err != nil {
		return ModelRecord{}, err
	} else if ok {
		if existing.State != modelStateReady {
			return ModelRecord{}, fmt.Errorf(
				"existing managed copy %q is %s; remove it before importing this model again",
				existing.ID,
				existing.State,
			)
		}
		if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
			return ModelRecord{}, fmt.Errorf("remove duplicate model copy: %w", err)
		}
		return existing, nil
	}

	id := managedModelID(spec.DisplayName, digest)
	directory := filepath.Join(m.modelsDir, id)
	if _, err := os.Stat(directory); err == nil {
		return ModelRecord{}, fmt.Errorf("managed model directory %q already exists", id)
	} else if !os.IsNotExist(err) {
		return ModelRecord{}, fmt.Errorf("inspect managed model directory: %w", err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return ModelRecord{}, fmt.Errorf("create managed model directory: %w", err)
	}
	modelPath := filepath.Join(directory, "model.gguf")
	if err := os.Rename(temporary, modelPath); err != nil {
		return ModelRecord{}, cleanupImportDirectory(directory, fmt.Errorf("commit managed model file: %w", err))
	}

	now := nowText()
	record := ModelRecord{
		ID: id, DisplayName: spec.DisplayName, Provider: "llama_cpp",
		Source: spec.Source, SourceName: spec.SourceName, Format: firstNonEmpty(spec.Format, "gguf"),
		Family: spec.Family, ParameterSize: spec.ParameterSize, Quantization: spec.Quantization,
		SizeBytes: size, SHA256: digest, ModelPath: modelPath, License: spec.License,
		ImportedAt: now, UpdatedAt: now, State: modelStateReady,
	}
	if err := writeModelMetadata(filepath.Join(directory, "metadata.json"), record); err != nil {
		return ModelRecord{}, cleanupImportDirectory(directory, err)
	}
	if err := m.insertModel(ctx, record); err != nil {
		return ModelRecord{}, cleanupImportDirectory(directory, fmt.Errorf("record managed model: %w", err))
	}
	return record, nil
}

func cleanupImportDirectory(directory string, cause error) error {
	if err := os.RemoveAll(directory); err != nil {
		return errors.Join(cause, fmt.Errorf("clean up failed model import: %w", err))
	}
	return cause
}

func cleanupImportPartial(path string, cause error) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.Join(cause, fmt.Errorf("clean up partial model import: %w", err))
	}
	return cause
}

func (m *ModelManager) updateImportProgress(id, status string, copied int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[id]; ok {
		job.snapshot.Status = status
		job.snapshot.BytesCopied = copied
		job.snapshot.UpdatedAt = nowText()
	}
}

func (m *ModelManager) finishImport(id, modelID string, importErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return
	}
	job.cancel = nil
	job.snapshot.UpdatedAt = nowText()
	if importErr == nil {
		job.snapshot.Status = ImportStatusComplete
		job.snapshot.BytesCopied = job.snapshot.TotalBytes
		job.snapshot.ModelID = modelID
		return
	}
	if errors.Is(importErr, context.Canceled) {
		job.snapshot.Status = ImportStatusCancelled
		job.snapshot.Error = "import cancelled"
		return
	}
	job.snapshot.Status = ImportStatusFailed
	job.snapshot.Error = importErr.Error()
}

func (m *ModelManager) completedImport(candidate OllamaCandidate) (ImportJob, error) {
	id, err := randomImportID()
	if err != nil {
		return ImportJob{}, err
	}
	now := nowText()
	job := &modelImportJob{snapshot: ImportJob{
		ID: id, Source: ModelSourceOllama, DisplayName: candidate.Name,
		Status: ImportStatusComplete, BytesCopied: candidate.SizeBytes,
		TotalBytes: candidate.SizeBytes, ModelID: candidate.ImportedModelID,
		StartedAt: now, UpdatedAt: now,
	}}
	m.mu.Lock()
	m.pruneImportJobsLocked()
	m.jobs[id] = job
	m.mu.Unlock()
	return job.snapshot, nil
}

func (m *ModelManager) pruneImportJobsLocked() {
	for len(m.jobs) >= maxTrackedImportJobs {
		oldestID := ""
		oldestUpdated := ""
		for id, job := range m.jobs {
			if activeImportStatus(job.snapshot.Status) {
				continue
			}
			if oldestID == "" || job.snapshot.UpdatedAt < oldestUpdated {
				oldestID = id
				oldestUpdated = job.snapshot.UpdatedAt
			}
		}
		if oldestID == "" {
			return
		}
		delete(m.jobs, oldestID)
	}
}

func validateImportSpec(spec modelImportSpec) error {
	displayName := strings.TrimSpace(spec.DisplayName)
	if displayName == "" {
		return errors.New("model display name is required")
	}
	if utf8.RuneCountInString(displayName) > maxModelNameChars {
		return fmt.Errorf("model display name must be at most %d characters", maxModelNameChars)
	}
	for _, char := range displayName {
		if unicode.IsControl(char) {
			return errors.New("model display name must not contain control characters")
		}
	}
	if spec.Source != ModelSourceGGUF && spec.Source != ModelSourceOllama {
		return fmt.Errorf("unsupported model source %q", spec.Source)
	}
	if spec.SourcePath == "" {
		return errors.New("model source path is required")
	}
	if spec.SizeBytes <= 4 || spec.SizeBytes > maxManagedModelBytes {
		return fmt.Errorf("model size must be between 5 bytes and %d bytes", maxManagedModelBytes)
	}
	if spec.ExpectedSHA != "" {
		digest := strings.TrimSpace(spec.ExpectedSHA)
		if len(digest) != sha256.Size*2 {
			return errors.New("expected model SHA-256 is invalid")
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return errors.New("expected model SHA-256 is invalid")
		}
	}
	return nil
}

func normalizeImportSpec(spec modelImportSpec) modelImportSpec {
	spec.DisplayName = strings.TrimSpace(spec.DisplayName)
	spec.Source = strings.TrimSpace(spec.Source)
	spec.SourceName = strings.TrimSpace(spec.SourceName)
	spec.ExpectedSHA = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(spec.ExpectedSHA), "sha256:"))
	spec.Format = strings.TrimSpace(spec.Format)
	spec.Family = strings.TrimSpace(spec.Family)
	spec.ParameterSize = strings.TrimSpace(spec.ParameterSize)
	spec.Quantization = strings.TrimSpace(spec.Quantization)
	spec.License = strings.TrimSpace(spec.License)
	return spec
}

func validateGGUFFile(path string) (os.FileInfo, error) {
	file, err := os.Open(path) // #nosec G304 -- importing this explicit local user-selected path is the endpoint's purpose.
	if err != nil {
		return nil, fmt.Errorf("open GGUF model: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("GGUF model path must be a regular file")
	}
	if info.Size() <= 4 || info.Size() > maxManagedModelBytes {
		return nil, fmt.Errorf("GGUF model size must be between 5 bytes and %d bytes", maxManagedModelBytes)
	}
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil || string(magic[:]) != "GGUF" {
		return nil, errors.New("selected file is not a GGUF model")
	}
	return info, nil
}

func managedModelID(name, digest string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(name) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if char <= unicode.MaxASCII {
				builder.WriteRune(char)
				lastDash = false
			}
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
		if builder.Len() >= 36 {
			break
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "model"
	}
	return slug + "-" + digest[:12]
}

func randomImportID() (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create import ID: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

func activeImportStatus(status string) bool {
	return status == ImportStatusQueued || status == ImportStatusCopying
}
