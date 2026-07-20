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
	"strings"
	"sync"
	"time"
)

const (
	// DatabaseFileName is the single app datastore under the resolved data dir.
	DatabaseFileName = "magichandy.db"

	// CurrentSchemaVersion is mirrored into PRAGMA user_version.
	CurrentSchemaVersion = 12

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

// ErrInvalidSchema reports a logically invalid schema that must not be
// replaced automatically because its rows may still be recoverable.
var ErrInvalidSchema = errors.New("sqlite schema is invalid")

const (
	maxOpenConnections = 4
	maxIdleConnections = 1
)

// DB wraps the process-owned SQLite connection and schema helpers.
type DB struct {
	sql *sql.DB

	writeMu  sync.Mutex
	recovery RecoveryStatus

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
	if err := secureDataDirectory(absDir); err != nil {
		return nil, err
	}

	path := filepath.Join(absDir, DatabaseFileName)
	return openDatastore(absDir, path)
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(ON)")
	values.Add("_pragma", "synchronous(NORMAL)")
	values.Add("_pragma", "cache_size(-2000)")
	return path + "?" + values.Encode()
}

// SQL exposes the underlying handle for package-owned reads. Production writes
// must use WithTx so every logical domain shares the serialized writer.
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

// Recovery reports whether Open quarantined a physically corrupt datastore
// and created a new one for this process.
func (db *DB) Recovery() RecoveryStatus {
	return db.recovery
}

// WithTx runs fn in one SQL transaction.
func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if fn == nil {
		return errors.New("SQLite transaction callback is required")
	}
	db.writeMu.Lock()
	defer db.writeMu.Unlock()

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
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
	var journalMode string
	if err := db.sql.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return fmt.Errorf("configure SQLite WAL: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("configure SQLite WAL: journal mode is %q", journalMode)
	}
	return nil
}

// migrations are forward-only steps; index i migrates user_version i to i+1.
// Never edit a shipped step — append a new one.
var migrations = [][]string{
	// v0 -> v1: the Phase 11B foundation tables.
	{
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
	},
	// v1 -> v2: the shared chat message log with per-client cursors
	// (ADR 0003 delivery ordering; ADR 0008 planned these tables).
	{
		`CREATE TABLE IF NOT EXISTS messages (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			client_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS client_cursors (
			client_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
	},
	// v2 -> v3: Phase 14 motion content, programs, and reversible feedback.
	{
		`CREATE TABLE IF NOT EXISTS patterns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			origin TEXT NOT NULL CHECK (origin IN ('builtin', 'user', 'generated')),
			kind TEXT NOT NULL CHECK (kind IN ('routine', 'burst')),
			enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
			weight REAL NOT NULL CHECK (weight >= 0.1 AND weight <= 3.0),
			cycle_ms INTEGER NOT NULL CHECK (cycle_ms > 0),
			points_json TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS patterns_enabled_weight
			ON patterns(enabled, weight DESC, name)`,
		`CREATE TABLE IF NOT EXISTS programs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			origin TEXT NOT NULL CHECK (origin IN ('user', 'imported')),
			duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
			points_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pattern_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern_id TEXT NOT NULL REFERENCES patterns(id) ON DELETE CASCADE,
			rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),
			weight_before REAL NOT NULL,
			weight_after REAL NOT NULL,
			enabled_before INTEGER NOT NULL CHECK (enabled_before IN (0, 1)),
			enabled_after INTEGER NOT NULL CHECK (enabled_after IN (0, 1)),
			reverted INTEGER NOT NULL DEFAULT 0 CHECK (reverted IN (0, 1)),
			created_at TEXT NOT NULL,
			reverted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS pattern_feedback_pattern_created
			ON pattern_feedback(pattern_id, id DESC)`,
	},
	// v3 -> v7 are reserved for the divergent Rockfire schema lineage. Those
	// builds reached user_version 7 before returning to the main architecture.
	// Keeping the version numbers prevents a Rockfire database from being
	// rejected as newer while the v8 reconciliation below repairs its core
	// tables without deleting the still-unimported LSO content.
	{`SELECT 1`},
	{`SELECT 1`},
	{`SELECT 1`},
	{`SELECT 1`},
	// v7 -> v8: reconcile both the main and Rockfire lineages. Every statement
	// is idempotent because a database may have reached version 7 through either
	// branch with a different subset of tables.
	{
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
		`CREATE TABLE IF NOT EXISTS messages (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			client_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS client_cursors (
			client_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS patterns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			origin TEXT NOT NULL CHECK (origin IN ('builtin', 'user', 'generated')),
			kind TEXT NOT NULL CHECK (kind IN ('routine', 'burst')),
			enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
			weight REAL NOT NULL CHECK (weight >= 0.1 AND weight <= 3.0),
			cycle_ms INTEGER NOT NULL CHECK (cycle_ms > 0),
			points_json TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS patterns_enabled_weight
			ON patterns(enabled, weight DESC, name)`,
		`CREATE TABLE IF NOT EXISTS programs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			origin TEXT NOT NULL CHECK (origin IN ('user', 'imported')),
			duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
			points_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pattern_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern_id TEXT NOT NULL REFERENCES patterns(id) ON DELETE CASCADE,
			rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),
			weight_before REAL NOT NULL,
			weight_after REAL NOT NULL,
			enabled_before INTEGER NOT NULL CHECK (enabled_before IN (0, 1)),
			enabled_after INTEGER NOT NULL CHECK (enabled_after IN (0, 1)),
			reverted INTEGER NOT NULL DEFAULT 0 CHECK (reverted IN (0, 1)),
			created_at TEXT NOT NULL,
			reverted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS pattern_feedback_pattern_created
			ON pattern_feedback(pattern_id, id DESC)`,
	},
	// v8 -> v9: managed local LLM model inventory. Model files remain in the
	// app data model store; SQLite owns searchable metadata and import lineage.
	{
		`CREATE TABLE IF NOT EXISTS llm_models (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			provider TEXT NOT NULL CHECK (provider = 'llama_cpp'),
			source TEXT NOT NULL CHECK (source IN ('gguf', 'ollama')),
			source_name TEXT NOT NULL DEFAULT '',
			format TEXT NOT NULL DEFAULT 'gguf',
			family TEXT NOT NULL DEFAULT '',
			parameter_size TEXT NOT NULL DEFAULT '',
			quantization TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
			sha256 TEXT NOT NULL,
			model_path TEXT NOT NULL,
			license TEXT NOT NULL DEFAULT '',
			imported_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS llm_models_sha256 ON llm_models(sha256)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS llm_models_path ON llm_models(model_path)`,
	},
	// v9 -> v10: preserve malformed or oversized settings documents before
	// the app activates defaults. These rows can contain credentials and stay
	// inside the same private datastore rather than a diagnostics export.
	{
		`CREATE TABLE IF NOT EXISTS settings_recoveries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			document TEXT NOT NULL,
			reason TEXT NOT NULL,
			recovered_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS settings_recoveries_recovered_at
			ON settings_recoveries(recovered_at DESC, id DESC)`,
	},
	// v10 -> v11: explicit-scan video catalog. Media files stay outside the
	// datastore; rows contain only bounded discovery metadata and jailed paths.
	{
		`CREATE TABLE IF NOT EXISTS media_videos (
			id TEXT PRIMARY KEY,
			location_path TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			display_name TEXT NOT NULL,
			size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
			modified_at TEXT NOT NULL,
			duration_ms INTEGER CHECK (duration_ms IS NULL OR duration_ms >= 0),
			funscript_relative_path TEXT,
			missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),
			scanned_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS media_videos_location_relative
			ON media_videos(location_path, relative_path)`,
		`CREATE INDEX IF NOT EXISTS media_videos_missing_name
			ON media_videos(missing, display_name, id)`,
	},
	// v11 -> v12: durable chat sessions. Existing global history is retained
	// as one saved conversation; per-session cursors prevent one tab from
	// consuming another conversation's tail. Run diagnostics are bounded JSON
	// attached to the visible assistant row, never a prompt or credential dump.
	{`SELECT 1`},
}

func (db *DB) migrate(ctx context.Context) error {
	if len(migrations) != CurrentSchemaVersion {
		return fmt.Errorf("%w: binary defines %d migrations for schema v%d", ErrInvalidSchema, len(migrations), CurrentSchemaVersion)
	}
	version, err := db.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version < 0 {
		return fmt.Errorf("%w: database version %d is negative", ErrInvalidSchema, version)
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: database version %d, binary version %d", ErrNewerSchema, version, CurrentSchemaVersion)
	}
	if version >= 8 {
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			return reconcileRockfireSchema(ctx, tx)
		}); err != nil {
			return fmt.Errorf("%w: reconcile schema v%d: %w", ErrInvalidSchema, version, err)
		}
	}
	if version == CurrentSchemaVersion {
		return nil
	}

	for next := version; next < CurrentSchemaVersion; next++ {
		step := migrations[next]
		err := db.WithTx(ctx, func(tx *sql.Tx) error {
			for _, statement := range step {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return fmt.Errorf("apply SQLite migration to v%d: %w", next+1, err)
				}
			}
			if next+1 == 8 {
				if err := reconcileRockfireSchema(ctx, tx); err != nil {
					return fmt.Errorf("reconcile SQLite schema at v%d: %w", next+1, err)
				}
			}
			if next+1 == 12 {
				if err := migrateChatSessions(ctx, tx); err != nil {
					return fmt.Errorf("migrate chat sessions at v%d: %w", next+1, err)
				}
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", next+1)); err != nil {
				return fmt.Errorf("record SQLite schema version %d: %w", next+1, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// migrateChatSessions is deliberately introspective. Migration tests and the
// Rockfire reconciliation can present a database whose user_version was
// rewound while some newer tables remain; treating that as an ordinary v11
// file must not duplicate or destroy those rows.
func migrateChatSessions(ctx context.Context, tx *sql.Tx) error {
	activeID, err := ensureChatWorkspace(ctx, tx)
	if err != nil {
		return err
	}
	return migrateChatSessionRows(ctx, tx, activeID)
}

func ensureChatWorkspace(ctx context.Context, tx *sql.Tx) (string, error) {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS chat_sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			saved INTEGER NOT NULL DEFAULT 0 CHECK (saved IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS chat_workspace (
			id TEXT PRIMARY KEY CHECK (id = 'current'),
			active_session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE RESTRICT,
			updated_at TEXT NOT NULL
		)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return "", err
		}
	}

	var activeID string
	err := tx.QueryRowContext(ctx, `
		SELECT s.id
		FROM chat_workspace w JOIN chat_sessions s ON s.id = w.active_session_id
		WHERE w.id = 'current'
	`).Scan(&activeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if activeID == "" {
		err = tx.QueryRowContext(ctx, `SELECT id FROM chat_sessions ORDER BY updated_at DESC, id DESC LIMIT 1`).Scan(&activeID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if activeID == "" {
		activeID = "legacy"
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_sessions(id, title, saved, created_at, updated_at)
			SELECT ?,
				CASE WHEN EXISTS(SELECT 1 FROM messages) THEN 'Previous conversation' ELSE 'New chat' END,
				CASE WHEN EXISTS(SELECT 1 FROM messages) THEN 1 ELSE 0 END,
				COALESCE((SELECT MIN(created_at) FROM messages), ?),
				COALESCE((SELECT MAX(created_at) FROM messages), ?)
		`, activeID, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chat_workspace(id, active_session_id, updated_at)
		VALUES('current', ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			active_session_id = excluded.active_session_id,
			updated_at = excluded.updated_at
	`, activeID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	return activeID, nil
}

func migrateChatSessionRows(ctx context.Context, tx *sql.Tx, activeID string) error {
	hasSessionID, err := columnExists(ctx, tx, "messages", "session_id")
	if err != nil {
		return err
	}
	if !hasSessionID {
		for _, statement := range []string{
			`ALTER TABLE messages RENAME TO messages_v11`,
			`CREATE TABLE messages (
				seq INTEGER PRIMARY KEY AUTOINCREMENT,
				session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
				role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
				content TEXT NOT NULL,
				client_id TEXT NOT NULL DEFAULT '',
				diagnostics_json TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL
			)`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO messages(seq, session_id, role, content, client_id, diagnostics_json, created_at)
			SELECT seq, ?, role, content, client_id, '{}', created_at FROM messages_v11
		`, activeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE messages_v11`); err != nil {
			return err
		}
	}

	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS messages_session_seq ON messages(session_id, seq)`,
		`CREATE INDEX IF NOT EXISTS chat_sessions_saved_updated ON chat_sessions(saved DESC, updated_at DESC, id)`,
		`CREATE TABLE IF NOT EXISTS chat_session_cursors (
			client_id TEXT NOT NULL,
			session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
			last_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(client_id, session_id)
		)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO chat_session_cursors(client_id, session_id, last_seq, updated_at)
		SELECT client_id, ?, last_seq, updated_at FROM client_cursors
	`, activeID); err != nil {
		return err
	}
	return nil
}

// reconcileRockfireSchema repairs the two table-shape differences created by
// the unmerged Rockfire branch. Its motion_blocks, funscript_files, queues,
// personas, and UI tables are deliberately left intact for the explicit LSO
// data-import phase; Phase 14 must not guess how to reinterpret that content.
func reconcileRockfireSchema(ctx context.Context, tx *sql.Tx) error {
	integerSettingsID, err := columnHasType(ctx, tx, "settings", "id", "INTEGER")
	if err != nil {
		return err
	}
	if integerSettingsID {
		var document, updatedAt string
		err := tx.QueryRowContext(ctx, `SELECT document, updated_at FROM settings WHERE id = 1`).Scan(
			&document,
			&updatedAt,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read Rockfire settings row: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE settings RENAME TO settings_rockfire_legacy`); err != nil {
			return fmt.Errorf("rename Rockfire settings table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE settings (
				id TEXT PRIMARY KEY CHECK (id = 'current'),
				document TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)
		`); err != nil {
			return fmt.Errorf("create canonical settings table: %w", err)
		}
		if document != "" {
			if updatedAt == "" {
				updatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settings(id, document, updated_at) VALUES('current', ?, ?)
			`, document, updatedAt); err != nil {
				return fmt.Errorf("copy Rockfire settings row: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE settings_rockfire_legacy`); err != nil {
			return fmt.Errorf("drop migrated Rockfire settings table: %w", err)
		}
	}

	hasPromptTimestamp, err := columnExists(ctx, tx, "prompt_sets", "created_at")
	if err != nil {
		return err
	}
	if !hasPromptTimestamp {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE prompt_sets ADD COLUMN created_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add prompt set timestamp: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE prompt_sets SET created_at = ? WHERE created_at = ''
		`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("backfill prompt set timestamp: %w", err)
		}
	}
	return nil
}

func columnHasType(ctx context.Context, tx *sql.Tx, table, column, columnType string) (bool, error) {
	typeName, exists, err := tableColumnType(ctx, tx, table, column)
	return exists && strings.EqualFold(typeName, columnType), err
}

func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	_, exists, err := tableColumnType(ctx, tx, table, column)
	return exists, err
}

func tableColumnType(ctx context.Context, tx *sql.Tx, table, column string) (string, bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table)) // #nosec G201 -- internal constants only.
	if err != nil {
		return "", false, fmt.Errorf("inspect %s table: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typeName string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &primaryKey); err != nil {
			return "", false, fmt.Errorf("scan %s table metadata: %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return typeName, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("read %s table metadata: %w", table, err)
	}
	return "", false, nil
}

func (db *DB) schemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read SQLite schema version: %w", err)
	}
	return version, nil
}
