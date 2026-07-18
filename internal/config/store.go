package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	settingsFileName         = "settings.json"
	maxSettingsDocumentBytes = 256 << 10
	maxSettingsRecoveries    = 20

	loadSourceDefault = "default"
	loadSourceSQLite  = "sqlite"
	loadSourceImport  = "imported_legacy_json"
	loadSourceRecover = "recovered_default"
)

var errSettingsDocumentTooLarge = errors.New("settings document exceeds the size limit")

// LoadStatus describes how settings were loaded without exposing setting values.
type LoadStatus struct {
	DataDir                string `json:"data_dir"`
	SettingsPath           string `json:"settings_path"`
	DatastorePath          string `json:"datastore_path"`
	DatastoreRecoveredPath string `json:"datastore_recovered_path,omitempty"`
	LegacySettingsPath     string `json:"legacy_settings_path,omitempty"`
	LegacyArchivedPath     string `json:"legacy_archived_path,omitempty"`
	Source                 string `json:"source"`
	UsingDefaults          bool   `json:"using_defaults"`
	Recovered              bool   `json:"recovered"`
	Migrated               bool   `json:"migrated"`
	Imported               bool   `json:"imported,omitempty"`
	Message                string `json:"message,omitempty"`
	LoadedAt               string `json:"loaded_at"`
}

// Store owns the process-local settings snapshot and durable settings row.
type Store struct {
	mu         sync.RWMutex
	dataDir    string
	path       string
	legacyPath string
	db         *dbstore.DB
	dbRecovery dbstore.RecoveryStatus
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
		dbRecovery: database.Recovery(),
	}

	importStatus, importedStatus, err := store.importLegacySettings(context.Background())
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	store.settings, store.status, err = store.loadSettings(context.Background(), importStatus, importedStatus)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

// Snapshot returns the current settings and load status.
func (s *Store) Snapshot() (Settings, LoadStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneSettings(s.settings), s.status
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

	durable := cloneSettings(next)
	if err := s.writeSettings(context.Background(), durable); err != nil {
		return Settings{}, err
	}
	s.settings = durable
	s.status = s.applyDatastoreRecovery(LoadStatus{
		DataDir:            s.dataDir,
		SettingsPath:       s.path,
		DatastorePath:      s.path,
		LegacySettingsPath: s.legacyPath,
		Source:             loadSourceSQLite,
		LoadedAt:           time.Now().UTC().Format(time.RFC3339Nano),
	})
	return cloneSettings(durable), nil
}

// DataDir returns the resolved settings directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// Path returns the durable settings file path.
func (s *Store) Path() string {
	return s.path
}

// Datastore returns the process-owned database for sibling runtime modules.
// Borrowers must not close it; Store.Close remains the single owner boundary.
func (s *Store) Datastore() *dbstore.DB {
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
	data, err := readSettingsDocument(s.legacyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			status.Status = dbstore.LegacyStatusRecovered
			if errors.Is(err, errSettingsDocumentTooLarge) {
				status.Message = "legacy settings file exceeded the size limit; defaults are active"
			} else {
				status.Message = "legacy settings file could not be read; defaults are active"
			}
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

func (s *Store) loadSettings(
	ctx context.Context,
	importStatus dbstore.LegacyImportStatus,
	importedStatus bool,
) (Settings, LoadStatus, error) {
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
	var documentBytes int64
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT
			length(CAST(document AS BLOB)),
			CASE WHEN length(CAST(document AS BLOB)) <= ? THEN document ELSE '' END
		FROM settings
		WHERE id = 'current'
	`, maxSettingsDocumentBytes).Scan(&documentBytes, &document)
	if err != nil {
		status.Source = loadSourceDefault
		status.UsingDefaults = true
		if importedStatus && importStatus.Status == dbstore.LegacyStatusRecovered {
			status.Source = loadSourceRecover
			status.Recovered = true
			status.Message = importStatus.Message
		} else if !isNoRows(err) {
			return Settings{}, LoadStatus{}, fmt.Errorf("read settings row: %w", err)
		} else if reason, recoveredAt, ok, recoveryErr := s.latestSettingsRecovery(ctx); recoveryErr != nil {
			return Settings{}, LoadStatus{}, recoveryErr
		} else if ok {
			status.Source = loadSourceRecover
			status.Recovered = true
			status.Message = fmt.Sprintf(
				"settings recovery from %s remains active (%s); defaults are active until settings are saved",
				recoveredAt,
				reason,
			)
		}
		return DefaultSettings(), s.applyDatastoreRecovery(status), nil
	}

	if documentBytes > maxSettingsDocumentBytes {
		if err := s.recoverSettingsDocument(ctx, "settings document exceeded the size limit"); err != nil {
			return Settings{}, LoadStatus{}, err
		}
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "oversized settings were preserved in recovery history; defaults are active"
		return DefaultSettings(), s.applyDatastoreRecovery(status), nil
	}

	settings, migrated, err := loadSettingsFromBytes([]byte(document))
	if err != nil {
		if recoveryErr := s.recoverSettingsDocument(ctx, "settings document could not be parsed"); recoveryErr != nil {
			return Settings{}, LoadStatus{}, recoveryErr
		}
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "invalid settings were preserved in recovery history; defaults are active"
		return DefaultSettings(), s.applyDatastoreRecovery(status), nil
	}
	if migrated {
		if err := s.writeSettings(ctx, settings); err != nil {
			return Settings{}, LoadStatus{}, fmt.Errorf("persist migrated settings: %w", err)
		}
	}

	status.Source = loadSourceSQLite
	status.Migrated = migrated
	if importedStatus && importStatus.Status == dbstore.LegacyStatusImported {
		status.Source = loadSourceImport
		status.Imported = true
		status.Message = importStatus.Message
	}
	return settings, s.applyDatastoreRecovery(status), nil
}

func (s *Store) recoverSettingsDocument(ctx context.Context, reason string) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO settings_recoveries(document, reason, recovered_at)
			SELECT document, ?, ? FROM settings WHERE id = 'current'
		`, reason, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("preserve invalid settings: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("confirm invalid settings preservation: %w", err)
		}
		if affected != 1 {
			return fmt.Errorf("preserve invalid settings: copied %d rows, want 1", affected)
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM settings_recoveries
			WHERE id NOT IN (
				SELECT id FROM settings_recoveries ORDER BY id DESC LIMIT ?
			)
		`, maxSettingsRecoveries); err != nil {
			return fmt.Errorf("prune settings recovery history: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE id = 'current'`); err != nil {
			return fmt.Errorf("activate default settings after preservation: %w", err)
		}
		return nil
	})
}

func (s *Store) latestSettingsRecovery(ctx context.Context) (reason, recoveredAt string, ok bool, err error) {
	err = s.db.SQL().QueryRowContext(ctx, `
		SELECT reason, recovered_at
		FROM settings_recoveries
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&reason, &recoveredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("read settings recovery history: %w", err)
	}
	return reason, recoveredAt, true, nil
}

func (s *Store) applyDatastoreRecovery(status LoadStatus) LoadStatus {
	if !s.dbRecovery.Recovered {
		return status
	}
	status.Recovered = true
	status.DatastoreRecoveredPath = s.dbRecovery.BackupDir
	if status.Source == loadSourceDefault {
		status.Source = loadSourceRecover
		status.UsingDefaults = true
	}
	if status.Message == "" {
		status.Message = s.dbRecovery.Message
	} else {
		status.Message = s.dbRecovery.Message + "; " + status.Message
	}
	return status
}

func (s *Store) writeSettings(ctx context.Context, settings Settings) error {
	document, err := marshalSettingsDocument(settings)
	if err != nil {
		return err
	}
	if len(document) > maxSettingsDocumentBytes {
		return fmt.Errorf("%w: %d bytes (maximum %d)", errSettingsDocumentTooLarge, len(document), maxSettingsDocumentBytes)
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

func readSettingsDocument(path string) ([]byte, error) {
	file, err := os.Open(path) // #nosec G304 -- resolved legacy app settings file.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxSettingsDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSettingsDocumentBytes {
		return nil, errSettingsDocumentTooLarge
	}
	return data, nil
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
