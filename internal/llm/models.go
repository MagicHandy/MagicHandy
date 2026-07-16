package llm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	// ModelSourceGGUF identifies a user-selected standalone GGUF file.
	ModelSourceGGUF = "gguf"
	// ModelSourceOllama identifies a GGUF copied from an Ollama library.
	ModelSourceOllama = "ollama"

	modelStateReady   = "ready"
	modelStateMissing = "missing"
	modelStateChanged = "changed"
	stalePartialAge   = 24 * time.Hour
)

var (
	// ErrModelNotFound reports a missing managed model record.
	ErrModelNotFound = errors.New("managed model not found")
	// ErrModelSelected prevents deleting the model selected by settings.
	ErrModelSelected = errors.New("selected model cannot be removed")
	// ErrImportNotFound reports an unknown in-memory import job.
	ErrImportNotFound = errors.New("model import not found")
	// ErrModelInventoryUnavailable classifies internal inventory failures for API callers.
	ErrModelInventoryUnavailable = errors.New("managed model inventory is unavailable")
)

// ModelRecord is one managed llama.cpp-compatible model copy.
type ModelRecord struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Provider      string `json:"provider"`
	Source        string `json:"source"`
	SourceName    string `json:"source_name,omitempty"`
	Format        string `json:"format"`
	Family        string `json:"family,omitempty"`
	ParameterSize string `json:"parameter_size,omitempty"`
	Quantization  string `json:"quantization,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	ModelPath     string `json:"model_path"`
	License       string `json:"license,omitempty"`
	ImportedAt    string `json:"imported_at"`
	UpdatedAt     string `json:"updated_at"`
	State         string `json:"state"`
	Message       string `json:"message,omitempty"`
}

// ModelSnapshot is the backend-authoritative model-manager view.
type ModelSnapshot struct {
	Models    []ModelRecord `json:"models"`
	Imports   []ImportJob   `json:"imports"`
	StorePath string        `json:"store_path"`
}

// ModelManager owns durable model metadata and managed file imports.
type ModelManager struct {
	db           *dbstore.DB
	modelsDir    string
	downloadsDir string

	mu     sync.Mutex
	jobs   map[string]*modelImportJob
	closed bool
	wg     sync.WaitGroup

	inventoryMu sync.Mutex
}

// OpenModelManager opens the inventory and prepares private model directories.
func OpenModelManager(dataDir string) (*ModelManager, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open model inventory: %w", err)
	}
	manager := &ModelManager{
		db:           database,
		modelsDir:    filepath.Join(database.DataDir(), "models", "gguf"),
		downloadsDir: filepath.Join(database.DataDir(), "downloads"),
		jobs:         make(map[string]*modelImportJob),
	}
	for _, directory := range []string{manager.modelsDir, manager.downloadsDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("create model directory: %w", err)
		}
	}
	if err := manager.removeStalePartials(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := manager.reconcileModelDeletions(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return manager, nil
}

// Close cancels imports, waits for copy loops, and closes the inventory.
func (m *ModelManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	for _, job := range m.jobs {
		if job.cancel != nil {
			job.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
	return m.db.Close()
}

// Snapshot returns managed models and current/recent import jobs.
func (m *ModelManager) Snapshot(ctx context.Context) (ModelSnapshot, error) {
	models, err := m.List(ctx)
	if err != nil {
		return ModelSnapshot{}, err
	}
	return ModelSnapshot{
		Models:    models,
		Imports:   m.ImportJobs(),
		StorePath: filepath.Dir(m.modelsDir),
	}, nil
}

// List returns inventory rows with filesystem state checked cheaply by size.
func (m *ModelManager) List(ctx context.Context) ([]ModelRecord, error) {
	rows, err := m.db.SQL().QueryContext(ctx, `
		SELECT id, display_name, provider, source, source_name, format, family,
		       parameter_size, quantization, size_bytes, sha256, model_path,
		       license, imported_at, updated_at
		FROM llm_models
		ORDER BY display_name, id
	`)
	if err != nil {
		return nil, modelInventoryError("list managed models", err)
	}
	defer func() { _ = rows.Close() }()

	models := make([]ModelRecord, 0)
	for rows.Next() {
		record, scanErr := scanModelRecord(rows)
		if scanErr != nil {
			return nil, modelInventoryError("scan managed model", scanErr)
		}
		models = append(models, m.modelFileState(record))
	}
	if err := rows.Err(); err != nil {
		return nil, modelInventoryError("read managed models", err)
	}
	return models, nil
}

// Model returns one managed model by stable ID.
func (m *ModelManager) Model(ctx context.Context, id string) (ModelRecord, error) {
	record, err := scanModelRecord(m.db.SQL().QueryRowContext(ctx, `
		SELECT id, display_name, provider, source, source_name, format, family,
		       parameter_size, quantization, size_bytes, sha256, model_path,
		       license, imported_at, updated_at
		FROM llm_models WHERE id = ?
	`, strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return ModelRecord{}, ErrModelNotFound
	}
	if err != nil {
		return ModelRecord{}, modelInventoryError("read managed model", err)
	}
	return m.modelFileState(record), nil
}

func modelInventoryError(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrModelInventoryUnavailable, operation, err)
}

// Delete removes only a MagicHandy-owned model copy. The selected model is
// protected so a running/configured provider never loses its backing file.
func (m *ModelManager) Delete(ctx context.Context, id, selectedID string) error {
	m.inventoryMu.Lock()
	defer m.inventoryMu.Unlock()

	record, err := m.Model(ctx, id)
	if err != nil {
		return err
	}
	if selectedID != "" && record.ID == selectedID {
		return ErrModelSelected
	}
	modelDir, err := m.modelDirectory(record)
	if err != nil {
		return err
	}
	tombstone := modelDir + ".deleting"
	filesMoved := false
	if _, statErr := os.Lstat(modelDir); statErr == nil {
		if _, tombstoneErr := os.Lstat(tombstone); tombstoneErr == nil {
			return errors.New("managed model already has pending deletion files")
		} else if !os.IsNotExist(tombstoneErr) {
			return fmt.Errorf("inspect managed model deletion: %w", tombstoneErr)
		}
		if renameErr := os.Rename(modelDir, tombstone); renameErr != nil {
			return fmt.Errorf("quarantine managed model files: %w", renameErr)
		}
		filesMoved = true
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect managed model files: %w", statErr)
	}
	deleteErr := m.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, deleteErr := tx.ExecContext(ctx, `DELETE FROM llm_models WHERE id = ?`, record.ID)
		if deleteErr != nil {
			return deleteErr
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrModelNotFound
		}
		return nil
	})
	if deleteErr != nil {
		if filesMoved {
			if restoreErr := os.Rename(tombstone, modelDir); restoreErr != nil {
				return errors.Join(deleteErr, fmt.Errorf("restore managed model files: %w", restoreErr))
			}
		}
		return deleteErr
	}
	if filesMoved {
		if err := os.RemoveAll(tombstone); err != nil {
			return fmt.Errorf("model record deleted, but quarantined files could not be removed: %w", err)
		}
	}
	return nil
}

func (m *ModelManager) modelBySHA(ctx context.Context, digest string) (ModelRecord, bool, error) {
	record, err := scanModelRecord(m.db.SQL().QueryRowContext(ctx, `
		SELECT id, display_name, provider, source, source_name, format, family,
		       parameter_size, quantization, size_bytes, sha256, model_path,
		       license, imported_at, updated_at
		FROM llm_models WHERE sha256 = ?
	`, strings.TrimPrefix(strings.TrimSpace(digest), "sha256:")))
	if errors.Is(err, sql.ErrNoRows) {
		return ModelRecord{}, false, nil
	}
	if err != nil {
		return ModelRecord{}, false, err
	}
	return m.modelFileState(record), true, nil
}

func (m *ModelManager) insertModel(ctx context.Context, record ModelRecord) error {
	return m.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO llm_models(
				id, display_name, provider, source, source_name, format, family,
				parameter_size, quantization, size_bytes, sha256, model_path,
				license, imported_at, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, record.ID, record.DisplayName, record.Provider, record.Source,
			record.SourceName, record.Format, record.Family, record.ParameterSize,
			record.Quantization, record.SizeBytes, record.SHA256, record.ModelPath,
			record.License, record.ImportedAt, record.UpdatedAt)
		return err
	})
}

func (m *ModelManager) removeStalePartials() error {
	entries, err := os.ReadDir(m.downloadsDir)
	if err != nil {
		return fmt.Errorf("read model downloads directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "model-import-") || !strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect model import: %w", err)
		}
		if info.ModTime().After(time.Now().Add(-stalePartialAge)) {
			continue
		}
		if err := os.Remove(filepath.Join(m.downloadsDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale model import: %w", err)
		}
	}
	return nil
}

func (m *ModelManager) reconcileModelDeletions() error {
	entries, err := os.ReadDir(m.modelsDir)
	if err != nil {
		return fmt.Errorf("read managed model directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".deleting") {
			continue
		}
		tombstone := filepath.Join(m.modelsDir, entry.Name())
		record, err := readModelMetadata(filepath.Join(tombstone, "metadata.json"))
		if err != nil {
			return fmt.Errorf("validate managed model deletion %q: %w", entry.Name(), err)
		}
		expectedID := strings.TrimSuffix(entry.Name(), ".deleting")
		if record.ID != expectedID {
			return fmt.Errorf("managed model deletion %q has metadata for %q", entry.Name(), record.ID)
		}
		original, err := m.modelDirectory(record)
		if err != nil {
			return fmt.Errorf("validate managed model deletion %q: %w", entry.Name(), err)
		}
		var storedPath string
		err = m.db.SQL().QueryRow(`SELECT model_path FROM llm_models WHERE id = ?`, record.ID).Scan(&storedPath)
		if err == nil {
			if !samePath(storedPath, record.ModelPath) {
				return fmt.Errorf("managed model deletion %q disagrees with its inventory path", entry.Name())
			}
			if _, err := os.Lstat(original); err == nil {
				return fmt.Errorf("managed model deletion has both active and quarantined files: %s", original)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect managed model restore target: %w", err)
			}
			if err := os.Rename(tombstone, original); err != nil {
				return fmt.Errorf("restore interrupted managed model deletion: %w", err)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reconcile managed model deletion: %w", err)
		}
		if err := os.RemoveAll(tombstone); err != nil {
			return fmt.Errorf("remove committed managed model deletion: %w", err)
		}
	}
	return nil
}

type modelScanner interface {
	Scan(dest ...any) error
}

func scanModelRecord(scanner modelScanner) (ModelRecord, error) {
	var record ModelRecord
	err := scanner.Scan(
		&record.ID, &record.DisplayName, &record.Provider, &record.Source,
		&record.SourceName, &record.Format, &record.Family, &record.ParameterSize,
		&record.Quantization, &record.SizeBytes, &record.SHA256, &record.ModelPath,
		&record.License, &record.ImportedAt, &record.UpdatedAt,
	)
	return record, err
}

func (m *ModelManager) modelFileState(record ModelRecord) ModelRecord {
	if _, err := m.modelDirectory(record); err != nil {
		record.State = modelStateMissing
		record.Message = "model inventory path is invalid"
		return record
	}
	info, err := os.Lstat(record.ModelPath)
	switch {
	case err != nil:
		record.State = modelStateMissing
		record.Message = "model file is unavailable"
	case !info.Mode().IsRegular():
		record.State = modelStateMissing
		record.Message = "model path is not a regular file"
	case info.Size() != record.SizeBytes:
		record.State = modelStateChanged
		record.Message = "model file size changed after import"
	default:
		record.State = modelStateReady
	}
	return record
}

func (m *ModelManager) modelDirectory(record ModelRecord) (string, error) {
	id := strings.TrimSpace(record.ID)
	if id == "" || id != record.ID || filepath.Base(id) != id || id == "." || id == ".." {
		return "", errors.New("managed model ID is invalid")
	}
	directory := filepath.Join(m.modelsDir, id)
	if !pathWithin(m.modelsDir, directory) {
		return "", errors.New("managed model directory is outside the model store")
	}
	expectedPath := filepath.Join(directory, "model.gguf")
	if !samePath(record.ModelPath, expectedPath) {
		return "", errors.New("managed model path does not match its inventory ID")
	}
	return directory, nil
}

func samePath(left, right string) bool {
	relative, err := filepath.Rel(filepath.Clean(left), filepath.Clean(right))
	return err == nil && relative == "."
}

func writeModelMetadata(path string, record ModelRecord) error {
	record.State = ""
	record.Message = ""
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model metadata: %w", err)
	}
	temporary := path + ".partial"
	if err := os.WriteFile(temporary, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write model metadata: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("commit model metadata: %w", err)
	}
	return nil
}

func readModelMetadata(path string) (ModelRecord, error) {
	file, err := os.Open(path) // #nosec G304 -- path is an app-owned tombstone metadata file.
	if err != nil {
		return ModelRecord{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ModelRecord{}, err
	}
	if info.Size() > 64<<10 {
		return ModelRecord{}, errors.New("model metadata exceeds 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	var record ModelRecord
	if err := decoder.Decode(&record); err != nil {
		return ModelRecord{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ModelRecord{}, errors.New("model metadata contains trailing content")
	}
	return record, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || relative == "." {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
