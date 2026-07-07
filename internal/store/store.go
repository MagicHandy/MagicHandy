package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

const (
	// DBFileName is the durable datastore file in the app data directory.
	DBFileName = "magichandy.db"

	// SchemaVersion is the latest forward migration applied at open.
	SchemaVersion = 3

	busyTimeoutMillis = 5000
	cacheSizePages    = -8192 // ~8 MiB page cache
)

// ImportResult reports which legacy JSON files were imported on first open.
type ImportResult struct {
	SettingsImported   bool
	MemoriesImported   bool
	PromptSetsImported bool
}

// Any reports whether at least one JSON store was imported.
func (r ImportResult) Any() bool {
	return r.SettingsImported || r.MemoriesImported || r.PromptSetsImported
}

// DB is the process-local SQLite datastore for one app data directory.
type DB struct {
	dataDir string
	path    string
	sql     *sql.DB
	writeMu sync.Mutex
	import_ ImportResult
}

var (
	trackMu  sync.Mutex
	tracked  []*DB
)

// Open connects to magichandy.db under dataDir, runs migrations, and performs a
// one-time import when the database file is created and legacy JSON stores exist.
func Open(dataDir string) (*DB, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}

	dbPath := filepath.Join(absDir, DBFileName)
	created := false
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		created = true
	} else if err != nil {
		return nil, fmt.Errorf("stat database: %w", err)
	}

	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	db := &DB{
		dataDir: absDir,
		path:    dbPath,
		sql:     sqlDB,
	}
	if err := db.applyPragmas(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if created {
		db.import_, err = importLegacyJSON(absDir, db)
		if err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}

	trackMu.Lock()
	tracked = append(tracked, db)
	trackMu.Unlock()
	return db, nil
}

// DataDir returns the resolved app data directory.
func (db *DB) DataDir() string {
	return db.dataDir
}

// Path returns the magichandy.db file path.
func (db *DB) Path() string {
	return db.path
}

// ImportResult returns the one-time JSON import outcome from open (empty when
// the database already existed or no legacy files were present).
func (db *DB) ImportResult() ImportResult {
	return db.import_
}

// Close releases the database connection.
func (db *DB) Close() error {
	untrack(db)
	_, _ = db.sql.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return db.sql.Close()
}

// CloseAll releases every open datastore handle (for tests).
func CloseAll() {
	trackMu.Lock()
	defer trackMu.Unlock()
	for _, db := range tracked {
		_, _ = db.sql.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
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

func (db *DB) applyPragmas() error {
	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		fmt.Sprintf("PRAGMA cache_size=%d", cacheSizePages),
		fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMillis),
	}
	for _, pragma := range pragmas {
		if _, err := db.sql.Exec(pragma); err != nil {
			return fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	return nil
}

func (db *DB) withWrite(fn func(tx *sql.Tx) error) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()

	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
