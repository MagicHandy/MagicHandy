package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	settingsFileName = "settings.json"

	loadSourceDefault = "default"
	loadSourceSQLite  = "sqlite"
	loadSourceImport  = "imported_legacy_json"
	loadSourceRecover = "recovered_default"
)

// LoadStatus describes how settings were loaded without exposing setting values.
type LoadStatus struct {
	DataDir            string `json:"data_dir"`
	SettingsPath       string `json:"settings_path"`
	DatastorePath      string `json:"datastore_path"`
	LegacySettingsPath string `json:"legacy_settings_path,omitempty"`
	LegacyArchivedPath string `json:"legacy_archived_path,omitempty"`
	Source             string `json:"source"`
	UsingDefaults      bool   `json:"using_defaults"`
	Recovered          bool   `json:"recovered"`
	Migrated           bool   `json:"migrated"`
	Imported           bool   `json:"imported,omitempty"`
	Message            string `json:"message,omitempty"`
	LoadedAt           string `json:"loaded_at"`
}

// Store owns the process-local settings snapshot and durable settings row.
type Store struct {
	mu         sync.RWMutex
	dataDir    string
	path       string
	legacyPath string
	db         *dbstore.DB
	settings   Settings
	status     LoadStatus
}

// ResolveDataDir returns the configured app data directory.
func ResolveDataDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if fromEnv := os.Getenv("MAGICHANDY_DATA_DIR"); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "MagicHandy"), nil
}

// OpenStore loads settings from dataDir and falls back to defaults safely.
func OpenStore(dataDir string) (*Store, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}

	database, err := dbstore.Open(absDir)
	if err != nil {
		return nil, err
	}
	store := &Store{
		dataDir:    database.DataDir(),
		path:       database.Path(),
		legacyPath: filepath.Join(database.DataDir(), settingsFileName),
		db:         database,
	}

	importStatus, importedStatus, err := store.importLegacySettings(context.Background())
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	store.settings, store.status = store.loadSettings(context.Background(), importStatus, importedStatus)
	if merged, mergeErr := store.mergeLegacyLLMPaths(context.Background()); mergeErr == nil && merged {
		if store.status.Message != "" {
			store.status.Message += "; "
		}
		store.status.Message += "merged llm paths from settings.json"
	}
	return store, nil
}

// Snapshot returns the current settings and load status.
func (s *Store) Snapshot() (Settings, LoadStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.settings, s.status
}

// PublicSnapshot returns the current redacted settings and load status.
func (s *Store) PublicSnapshot() (PublicSettings, LoadStatus) {
	settings, status := s.Snapshot()
	return settings.Public(), status
}

// Save validates, writes, and publishes the next settings snapshot.
func (s *Store) Save(next Settings) (Settings, error) {
	next, err := NormalizeSettings(next)
	if err != nil {
		return Settings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeSettings(context.Background(), next); err != nil {
		return Settings{}, err
	}
	s.settings = next
	s.status = LoadStatus{
		DataDir:            s.dataDir,
		SettingsPath:       s.path,
		DatastorePath:      s.path,
		LegacySettingsPath: s.legacyPath,
		Source:             loadSourceSQLite,
		LoadedAt:           time.Now().UTC().Format(time.RFC3339Nano),
	}
	return next, nil
}

// DataDir returns the resolved settings directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// Path returns the durable settings file path.
func (s *Store) Path() string {
	return s.path
}

// DB exposes the embedded SQLite datastore for Rockfire UI and library features.
func (s *Store) DB() *dbstore.DB {
	return s.db
}

// Close releases the settings store database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) importLegacySettings(ctx context.Context) (dbstore.LegacyImportStatus, bool, error) {
	const domain = "settings"
	existing, ok, err := s.db.LegacyImportStatus(ctx, domain)
	if err != nil {
		return dbstore.LegacyImportStatus{}, false, fmt.Errorf("read settings import status: %w", err)
	}
	if ok {
		return existing, true, nil
	}

	status := dbstore.LegacyImportStatus{
		Domain:     domain,
		SourcePath: s.legacyPath,
		Status:     dbstore.LegacyStatusAbsent,
	}
	data, err := os.ReadFile(s.legacyPath) // #nosec G304 -- resolved legacy app settings file.
	if err != nil {
		if !os.IsNotExist(err) {
			status.Status = dbstore.LegacyStatusRecovered
			status.Message = "legacy settings file could not be read; defaults are active"
		}
		return status, true, s.db.RecordLegacyImport(ctx, status)
	}

	settings, migrated, err := loadSettingsFromBytes(data)
	if err != nil {
		status.Status = dbstore.LegacyStatusRecovered
		status.Message = "legacy settings file could not be parsed; defaults are active"
		return status, true, s.db.RecordLegacyImport(ctx, status)
	}
	document, err := marshalSettingsDocument(settings)
	if err != nil {
		return dbstore.LegacyImportStatus{}, false, err
	}

	status.Status = dbstore.LegacyStatusImported
	status.Message = "legacy settings imported"
	if migrated {
		status.Message = "legacy settings imported and migrated"
	}
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings(id, document, updated_at)
			VALUES('current', ?, ?)
			ON CONFLICT(id) DO NOTHING
		`, string(document), now); err != nil {
			return err
		}
		return dbstore.RecordLegacyImportTx(ctx, tx, status)
	}); err != nil {
		return dbstore.LegacyImportStatus{}, false, fmt.Errorf("import legacy settings: %w", err)
	}

	archivePath, archiveErr := dbstore.ArchiveLegacyJSON(s.legacyPath)
	if archiveErr != nil {
		status.Message += "; legacy settings archive failed: " + archiveErr.Error()
	} else if archivePath != "" {
		status.ArchivedPath = archivePath
	}
	if status.ArchivedPath != "" || archiveErr != nil {
		if err := s.db.RecordLegacyImport(ctx, status); err != nil {
			return dbstore.LegacyImportStatus{}, false, err
		}
	}
	return status, true, nil
}

func (s *Store) loadSettings(ctx context.Context, importStatus dbstore.LegacyImportStatus, importedStatus bool) (Settings, LoadStatus) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := LoadStatus{
		DataDir:            s.dataDir,
		SettingsPath:       s.path,
		DatastorePath:      s.path,
		LegacySettingsPath: s.legacyPath,
		LoadedAt:           now,
	}
	if importedStatus {
		status.LegacyArchivedPath = importStatus.ArchivedPath
	}

	var document string
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT document
		FROM settings
		WHERE id = 'current'
	`).Scan(&document)
	if err != nil {
		status.Source = loadSourceDefault
		status.UsingDefaults = true
		if importedStatus && importStatus.Status == dbstore.LegacyStatusRecovered {
			status.Source = loadSourceRecover
			status.Recovered = true
			status.Message = importStatus.Message
		} else if !isNoRows(err) {
			status.Source = loadSourceRecover
			status.Recovered = true
			status.Message = "settings row could not be read; defaults are active"
		}
		return DefaultSettings(), status
	}

	settings, migrated, err := loadSettingsFromBytes([]byte(document))
	if err != nil {
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "settings row could not be parsed; defaults are active"
		return DefaultSettings(), status
	}

	status.Source = loadSourceSQLite
	status.Migrated = migrated
	if importedStatus && importStatus.Status == dbstore.LegacyStatusImported {
		status.Source = loadSourceImport
		status.Imported = true
		status.Message = importStatus.Message
	}
	return settings, status
}

func (s *Store) writeSettings(ctx context.Context, settings Settings) error {
	document, err := marshalSettingsDocument(settings)
	if err != nil {
		return err
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO settings(id, document, updated_at)
			VALUES('current', ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				document = excluded.document,
				updated_at = excluded.updated_at
		`, string(document), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func marshalSettingsDocument(settings Settings) ([]byte, error) {
	settings, err := NormalizeSettings(settings)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return append(data, '\n'), nil
}

func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func (s *Store) mergeLegacyLLMPaths(ctx context.Context) (bool, error) {
	data, err := os.ReadFile(s.legacyPath) // #nosec G304 -- resolved legacy app settings file.
	if err != nil {
		return false, nil
	}
	legacy, _, err := loadSettingsFromBytes(data)
	if err != nil {
		return false, nil
	}

	s.mu.Lock()
	next := s.settings
	changed := false
	if next.LLM.LlamaCPPRunnerPath == "" && legacy.LLM.LlamaCPPRunnerPath != "" {
		next.LLM.LlamaCPPRunnerPath = legacy.LLM.LlamaCPPRunnerPath
		changed = true
	}
	if next.LLM.LlamaCPPModelPath == "" && legacy.LLM.LlamaCPPModelPath != "" {
		next.LLM.LlamaCPPModelPath = legacy.LLM.LlamaCPPModelPath
		changed = true
	}
	if legacy.LLM.LlamaCPPBaseURL != "" && next.LLM.LlamaCPPBaseURL != legacy.LLM.LlamaCPPBaseURL {
		next.LLM.LlamaCPPBaseURL = legacy.LLM.LlamaCPPBaseURL
		changed = true
	}
	if legacy.LLM.Model != "" && next.LLM.Model != legacy.LLM.Model {
		next.LLM.Model = legacy.LLM.Model
		changed = true
	}
	if !changed {
		s.mu.Unlock()
		return false, nil
	}

	normalized, err := NormalizeSettings(next)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	if err := s.writeSettings(ctx, normalized); err != nil {
		s.mu.Unlock()
		return false, err
	}
	s.settings = normalized
	s.mu.Unlock()
	return true, nil
}
