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

	// CurrentSchemaVersion is mirrored into PRAGMA user_version.
	CurrentSchemaVersion = 1

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

var writeMu sync.Mutex

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
	return db.sql.Close()
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
	if version == CurrentSchemaVersion {
		return nil
	}

	return db.WithTx(ctx, func(tx *sql.Tx) error {
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
			`PRAGMA user_version = 1`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply SQLite migration: %w", err)
			}
		}
		return nil
	})
}

func (db *DB) schemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read SQLite schema version: %w", err)
	}
	return version, nil
}
