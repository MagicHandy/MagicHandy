// Package store owns MagicHandy's embedded SQLite datastore, schema migrations,
// transaction helpers, and legacy JSON import bookkeeping.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // register the pure-Go SQLite database/sql driver
)

const (
	// DatabaseFileName is the single app datastore under the resolved data dir.
	DatabaseFileName = "magichandy.db"
	// DBFileName is a Rockfire-era alias kept for tests and tooling.
	DBFileName = DatabaseFileName

	// CurrentSchemaVersion is mirrored into PRAGMA user_version.
	CurrentSchemaVersion = 3
	// SchemaVersion is a Rockfire-era alias kept for tests.
	SchemaVersion = CurrentSchemaVersion

	// LegacyStatusAbsent records that a legacy JSON file was not present.
	LegacyStatusAbsent = "absent"
	// LegacyStatusImported records that a legacy JSON file imported successfully.
	LegacyStatusImported = "imported"
	// LegacyStatusRecovered records that a legacy JSON file was unreadable or corrupt.
	LegacyStatusRecovered = "recovered"
	// LegacyStatusSkipped records that import was intentionally skipped.
	LegacyStatusSkipped = "skipped"
)

// ErrNewerSchema reports a DB created by a newer MagicHandy binary.
var ErrNewerSchema = errors.New("sqlite schema is newer than this binary")

var (
	writeMu sync.Mutex
	trackMu sync.Mutex
	tracked []*DB
)

// DB wraps the shared SQLite file and schema helpers.
type DB struct {
	sql     *sql.DB
	dataDir string
	path    string
}

// LegacyImportStatus records one legacy JSON import attempt.
type LegacyImportStatus struct {
	Domain       string
	SourcePath   string
	ArchivedPath string
	Status       string
	Message      string
	ImportedAt   string
}

// Open opens the MagicHandy SQLite datastore and applies migrations.
func Open(dataDir string) (*DB, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	path := filepath.Join(absDir, DatabaseFileName)
	handle, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite datastore: %w", err)
	}
	handle.SetMaxOpenConns(1)
	handle.SetMaxIdleConns(0)

	db := &DB{sql: handle, dataDir: absDir, path: path}
	if err := db.configure(context.Background()); err != nil {
		_ = handle.Close()
		return nil, err
	}
	if err := db.migrate(context.Background()); err != nil {
		_ = handle.Close()
		return nil, err
	}

	trackMu.Lock()
	tracked = append(tracked, db)
	trackMu.Unlock()
	return db, nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(ON)")
	values.Add("_pragma", "synchronous(NORMAL)")
	values.Add("_pragma", "cache_size(-2000)")
	values.Add("_pragma", "journal_mode(WAL)")
	return path + "?" + values.Encode()
}

// SQL exposes the underlying database/sql handle for package-owned queries.
func (db *DB) SQL() *sql.DB {
	return db.sql
}

// DataDir returns the resolved app data directory.
func (db *DB) DataDir() string {
	return db.dataDir
}

// Path returns the SQLite database file path.
func (db *DB) Path() string {
	return db.path
}

// Close releases the database handle.
func (db *DB) Close() error {
	untrack(db)
	return db.sql.Close()
}

// CloseAll releases every open datastore handle (for tests).
func CloseAll() {
	trackMu.Lock()
	defer trackMu.Unlock()
	for _, db := range tracked {
		_ = db.sql.Close()
	}
	tracked = nil
}

func untrack(db *DB) {
	trackMu.Lock()
	defer trackMu.Unlock()
	for i, item := range tracked {
		if item == db {
			tracked = append(tracked[:i], tracked[i+1:]...)
			return
		}
	}
}

// WithTx runs fn in one SQL transaction.
func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// withWrite runs fn in one write transaction for Rockfire datastore helpers.
func (db *DB) withWrite(fn func(tx *sql.Tx) error) error {
	return db.WithTx(context.Background(), fn)
}

// LegacyImportStatus returns the recorded import state for a legacy JSON file.
func (db *DB) LegacyImportStatus(ctx context.Context, domain string) (LegacyImportStatus, bool, error) {
	var status LegacyImportStatus
	err := db.sql.QueryRowContext(ctx, `
		SELECT domain, source_path, archived_path, status, message, imported_at
		FROM legacy_imports
		WHERE domain = ?
	`, domain).Scan(
		&status.Domain,
		&status.SourcePath,
		&status.ArchivedPath,
		&status.Status,
		&status.Message,
		&status.ImportedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyImportStatus{}, false, nil
	}
	if err != nil {
		return LegacyImportStatus{}, false, err
	}
	return status, true, nil
}

// RecordLegacyImport records a one-time import status.
func (db *DB) RecordLegacyImport(ctx context.Context, status LegacyImportStatus) error {
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		return RecordLegacyImportTx(ctx, tx, status)
	})
}

// RecordLegacyImportTx records a one-time import status inside an existing tx.
func RecordLegacyImportTx(ctx context.Context, tx *sql.Tx, status LegacyImportStatus) error {
	if status.ImportedAt == "" {
		status.ImportedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO legacy_imports(domain, source_path, archived_path, status, message, imported_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			source_path = excluded.source_path,
			archived_path = excluded.archived_path,
			status = excluded.status,
			message = excluded.message,
			imported_at = excluded.imported_at
	`, status.Domain, status.SourcePath, status.ArchivedPath, status.Status, status.Message, status.ImportedAt)
	return err
}

// ArchiveLegacyJSON renames a successfully imported JSON file without deleting
// its contents. If the archive name already exists, the original is left alone.
func ArchiveLegacyJSON(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	archivePath := path + ".migrated"
	if _, err := os.Stat(archivePath); err == nil {
		return "", nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(path, archivePath); err != nil {
		return "", err
	}
	return archivePath, nil
}

func (db *DB) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -2000",
	}
	for _, pragma := range pragmas {
		if _, err := db.sql.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure SQLite %q: %w", pragma, err)
		}
	}
	if _, err := db.sql.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("configure SQLite WAL: %w", err)
	}
	return nil
}

func (db *DB) migrate(ctx context.Context) error {
	version, err := db.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: database version %d, binary version %d", ErrNewerSchema, version, CurrentSchemaVersion)
	}
	for version < CurrentSchemaVersion {
		version++
		next := version
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			switch next {
			case 1:
				if err := migrateV1(ctx, tx); err != nil {
					return err
				}
			case 2:
				if err := migrateV2(ctx, tx); err != nil {
					return err
				}
			case 3:
				if err := migrateV3(ctx, tx); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown migration version %d", next)
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", next)); err != nil {
				return fmt.Errorf("set user_version: %w", err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("migration to version %d: %w", next, err)
		}
	}
	return nil
}

func migrateV1(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id TEXT PRIMARY KEY CHECK (id = 'current'),
			document TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_sets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			system TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS legacy_imports (
			domain TEXT PRIMARY KEY,
			source_path TEXT NOT NULL,
			archived_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			imported_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply SQLite migration v1: %w", err)
		}
	}
	return nil
}

func migrateV2(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS funscript_files (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			path TEXT NOT NULL,
			duration_ms INTEGER,
			action_count INTEGER,
			imported_at TEXT,
			hash TEXT UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS motion_blocks (
			id TEXT PRIMARY KEY,
			source_file_id TEXT NOT NULL,
			start_ms INTEGER NOT NULL,
			end_ms INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			min_pos INTEGER,
			max_pos INTEGER,
			avg_pos REAL,
			amplitude INTEGER,
			zone TEXT,
			stroke_length TEXT,
			speed TEXT,
			rhythm TEXT,
			intensity REAL,
			tags_json TEXT,
			actions_json TEXT NOT NULL,
			user_rating INTEGER,
			times_used INTEGER NOT NULL DEFAULT 0,
			success_score REAL NOT NULL DEFAULT 0,
			blocked INTEGER NOT NULL DEFAULT 0,
			favorite INTEGER NOT NULL DEFAULT 0,
			created_at TEXT,
			FOREIGN KEY(source_file_id) REFERENCES funscript_files(id)
		)`,
		`CREATE TABLE IF NOT EXISTS saved_queues (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			items_json TEXT NOT NULL,
			actions_json TEXT,
			duration_ms INTEGER,
			funscript_file_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(funscript_file_id) REFERENCES funscript_files(id)
		)`,
		`CREATE TABLE IF NOT EXISTS personas (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			system_prompt TEXT NOT NULL,
			tone_json TEXT,
			mood_json TEXT,
			boundaries_json TEXT,
			motion_bias_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_prompt_sets_name ON prompt_sets(name)`,
		`CREATE INDEX IF NOT EXISTS idx_motion_blocks_source ON motion_blocks(source_file_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply SQLite migration v2: %w", err)
		}
	}
	return nil
}

func migrateV3(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ui_preferences (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			locale TEXT NOT NULL DEFAULT 'en',
			locale_prompt_dismissed INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO ui_preferences (id, locale, locale_prompt_dismissed, updated_at)
		 VALUES (1, 'en', 0, datetime('now'))`,
		`CREATE TABLE IF NOT EXISTS app_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			active_persona_id TEXT NOT NULL DEFAULT '',
			operation_mode TEXT NOT NULL DEFAULT 'hybrid',
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO app_state (id, active_persona_id, operation_mode, updated_at)
		 VALUES (1, '', 'hybrid', datetime('now'))`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply SQLite migration v3: %w", err)
		}
	}
	return nil
}

func (db *DB) schemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read SQLite schema version: %w", err)
	}
	return version, nil
}
