package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenCreatesSchemaAndDatabaseFile(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if _, err := os.Stat(db.Path()); err != nil {
		t.Fatalf("database file missing: %v", err)
	}
	var version int
	if err := db.SQL().QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, CurrentSchemaVersion)
	}
	stats := db.SQL().Stats()
	if stats.MaxOpenConnections != maxOpenConnections || stats.Idle != maxIdleConnections {
		t.Fatalf("connection pool stats = %+v, want max-open %d and one warm idle connection", stats, maxOpenConnections)
	}
}

func TestOpenRejectsNewerSchemaWithoutDowngrade(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 999"); err != nil {
		_ = raw.Close()
		t.Fatalf("set newer user_version: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	_, err = Open(dir)
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("Open newer schema error = %v, want ErrNewerSchema", err)
	}

	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw reopen: %v", err)
	}
	defer func() {
		_ = raw.Close()
	}()
	var version int
	if err := raw.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version after rejection: %v", err)
	}
	if version != 999 {
		t.Fatalf("user_version after rejection = %d, want 999", version)
	}
}

func TestOpenRejectsNegativeSchemaWithoutPanicking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DatabaseFileName)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = -1"); err != nil {
		_ = raw.Close()
		t.Fatalf("set negative user_version: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	if _, err := Open(dir); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("Open negative schema error = %v, want ErrInvalidSchema", err)
	}
}

func TestOpenRecoversPhysicalCorruptionWithoutDeletingOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DatabaseFileName)
	original := []byte("not a SQLite database\x00preserve these bytes")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write corrupt database: %v", err)
	}

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open corrupt database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	recovery := db.Recovery()
	if !recovery.Recovered || recovery.BackupDir == "" || recovery.Message == "" {
		t.Fatalf("recovery = %+v, want preserved corrupt datastore", recovery)
	}
	relative, err := filepath.Rel(dir, recovery.BackupDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("backup path %q is outside data dir %q", recovery.BackupDir, dir)
	}
	preserved, err := os.ReadFile(filepath.Join(recovery.BackupDir, DatabaseFileName)) // #nosec G304 -- app-reported temp recovery path.
	if err != nil {
		t.Fatalf("read preserved database: %v", err)
	}
	if !bytes.Equal(preserved, original) {
		t.Fatalf("preserved database bytes changed: %q", preserved)
	}
	var version int
	if err := db.SQL().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read replacement schema: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("replacement schema = %d, want %d", version, CurrentSchemaVersion)
	}
}

func TestOpenDoesNotReplaceLogicallyInvalidCurrentSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec("DROP TABLE memories"); err != nil {
		_ = raw.Close()
		t.Fatalf("drop required table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	if _, err := Open(dir); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("Open damaged schema error = %v, want ErrInvalidSchema", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "recovery")); !os.IsNotExist(err) {
		t.Fatalf("logical schema damage was quarantined as physical corruption: %v", err)
	}
}

func TestOpenRejectsMisdefinedRequiredIndex(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		`DROP INDEX llm_models_sha256`,
		`CREATE UNIQUE INDEX llm_models_sha256 ON llm_models(display_name)`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("alter required index with %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	if _, err := Open(dir); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("Open misdefined index error = %v, want ErrInvalidSchema", err)
	}
}

func TestOpenRejectsMissingRequiredForeignKey(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		`DROP TABLE pattern_feedback`,
		`CREATE TABLE pattern_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern_id TEXT NOT NULL,
			rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),
			weight_before REAL NOT NULL,
			weight_after REAL NOT NULL,
			enabled_before INTEGER NOT NULL CHECK (enabled_before IN (0, 1)),
			enabled_after INTEGER NOT NULL CHECK (enabled_after IN (0, 1)),
			reverted INTEGER NOT NULL DEFAULT 0 CHECK (reverted IN (0, 1)),
			created_at TEXT NOT NULL,
			reverted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX pattern_feedback_pattern_created ON pattern_feedback(pattern_id, id DESC)`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("alter required foreign key with %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	if _, err := Open(dir); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("Open missing foreign key error = %v, want ErrInvalidSchema", err)
	}
}

func TestWithTxRollsBackWhenCallbackPanics(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = db.WithTx(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO app_kv(key, value, updated_at) VALUES('panic', 'value', 'now')`); err != nil {
				t.Fatalf("insert before panic: %v", err)
			}
			panic("transaction fixture")
		})
	}()
	if recovered != "transaction fixture" {
		t.Fatalf("recovered panic = %#v", recovered)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM app_kv WHERE key = 'panic'`).Scan(&count); err != nil {
		t.Fatalf("query after panic: %v", err)
	}
	if count != 0 {
		t.Fatalf("panicking transaction committed %d row(s)", count)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO app_kv(key, value, updated_at) VALUES('after', 'value', 'now')`)
		return err
	}); err != nil {
		t.Fatalf("transaction after panic: %v", err)
	}
}

func TestOpenEnforcesPrivateDatastorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file privacy is enforced by profile ACLs, not POSIX mode bits")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil { // #nosec G302 -- fixture deliberately starts too permissive.
		t.Fatalf("relax fixture directory: %v", err)
	}
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("data dir mode = %o, want 700", got)
	}
	dbInfo, err := os.Stat(db.Path())
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

func TestArchiveLegacyJSONPreservesContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	contents := []byte(`{"version":1}`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	archivePath, err := ArchiveLegacyJSON(path)
	if err != nil {
		t.Fatalf("ArchiveLegacyJSON: %v", err)
	}
	if archivePath != path+".migrated" {
		t.Fatalf("archive path = %q, want %q", archivePath, path+".migrated")
	}
	archived, err := os.ReadFile(archivePath) // #nosec G304 -- temp test path.
	if err != nil {
		t.Fatalf("read archived file: %v", err)
	}
	if string(archived) != string(contents) {
		t.Fatalf("archived contents = %q, want %q", archived, contents)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("legacy path stat error = %v, want not exists", err)
	}
}

func TestMigrationUpgradesV1DatabaseInPlace(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	// Preserve a v1-era row so the upgrade provably keeps existing data.
	if _, err := db.SQL().Exec(
		`INSERT INTO memories(id, text, enabled, created_at) VALUES('m1', 'keep me', 1, '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Rewind the file to schema v1 by dropping the v2 tables.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		"DROP TABLE messages",
		"DROP TABLE client_cursors",
		"PRAGMA user_version = 1",
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v1 database: %v", err)
	}
	defer func() {
		_ = upgraded.Close()
	}()

	var version int
	if err := upgraded.SQL().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, CurrentSchemaVersion)
	}
	if _, err := upgraded.SQL().Exec(
		`INSERT INTO messages(role, content, client_id, created_at) VALUES('user', 'hi', 'c', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("v2 messages table missing after upgrade: %v", err)
	}
	var kept string
	if err := upgraded.SQL().QueryRow(`SELECT text FROM memories WHERE id = 'm1'`).Scan(&kept); err != nil || kept != "keep me" {
		t.Fatalf("v1 data lost across upgrade: %q, %v", kept, err)
	}
}

func TestMigrationUpgradesV2DatabaseToPatternSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		"DROP TABLE pattern_feedback",
		"DROP TABLE programs",
		"DROP TABLE patterns",
		"PRAGMA user_version = 2",
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v2 database: %v", err)
	}
	defer func() { _ = upgraded.Close() }()

	for _, table := range []string{"patterns", "programs", "pattern_feedback", "llm_models", "settings_recoveries"} {
		assertTableExists(t, upgraded.SQL(), table)
	}
	var version int
	if err := upgraded.SQL().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, CurrentSchemaVersion)
	}
}

func TestMigrationReconcilesRockfireV7WithoutDeletingLibraryData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DatabaseFileName)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	statements := []string{
		`CREATE TABLE settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			document TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO settings(id, document, updated_at)
		 VALUES(1, '{"version":1}', '2026-07-10T00:00:00Z')`,
		`CREATE TABLE prompt_sets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			system TEXT NOT NULL
		)`,
		`INSERT INTO prompt_sets(id, name, system) VALUES('p1', 'Keep', 'Preserve me')`,
		`CREATE TABLE motion_blocks (
			id TEXT PRIMARY KEY,
			actions_json TEXT NOT NULL
		)`,
		`INSERT INTO motion_blocks(id, actions_json) VALUES('b1', '[{"at":0,"pos":10}]')`,
		`PRAGMA user_version = 7`,
	}
	for _, statement := range statements {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("seed Rockfire schema with %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open Rockfire v7 database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var id, document, updatedAt string
	if err := db.SQL().QueryRow(`SELECT id, document, updated_at FROM settings`).Scan(&id, &document, &updatedAt); err != nil {
		t.Fatalf("read reconciled settings: %v", err)
	}
	if id != "current" || document != `{"version":1}` || updatedAt != "2026-07-10T00:00:00Z" {
		t.Fatalf("reconciled settings = (%q, %q, %q)", id, document, updatedAt)
	}

	var promptSystem, createdAt string
	if err := db.SQL().QueryRow(`SELECT system, created_at FROM prompt_sets WHERE id = 'p1'`).Scan(
		&promptSystem,
		&createdAt,
	); err != nil {
		t.Fatalf("read reconciled prompt set: %v", err)
	}
	if promptSystem != "Preserve me" || createdAt == "" {
		t.Fatalf("reconciled prompt set = (%q, %q)", promptSystem, createdAt)
	}

	var actions string
	if err := db.SQL().QueryRow(`SELECT actions_json FROM motion_blocks WHERE id = 'b1'`).Scan(&actions); err != nil {
		t.Fatalf("Rockfire motion block was not preserved: %v", err)
	}
	if actions != `[{"at":0,"pos":10}]` {
		t.Fatalf("motion block actions = %q", actions)
	}
	for _, table := range []string{"messages", "client_cursors", "patterns", "programs", "pattern_feedback", "llm_models"} {
		assertTableExists(t, db.SQL(), table)
	}
}

func TestMigrationReconcilesKnownRockfireShapesAtCurrentVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	statements := []string{
		`DROP TABLE settings`,
		`CREATE TABLE settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			document TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO settings(id, document, updated_at)
		 VALUES(1, '{"version":1}', '2026-07-18T00:00:00Z')`,
		`DROP TABLE prompt_sets`,
		`CREATE TABLE prompt_sets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			system TEXT NOT NULL
		)`,
		`INSERT INTO prompt_sets(id, name, system) VALUES('p-current', 'Keep', 'Current-version row')`,
		`PRAGMA user_version = 10`,
	}
	for _, statement := range statements {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("seed current Rockfire shape with %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	reconciled, err := Open(dir)
	if err != nil {
		t.Fatalf("Open current Rockfire shape: %v", err)
	}
	t.Cleanup(func() { _ = reconciled.Close() })
	var id, document string
	if err := reconciled.SQL().QueryRow(`SELECT id, document FROM settings`).Scan(&id, &document); err != nil {
		t.Fatalf("read reconciled settings: %v", err)
	}
	if id != "current" || document != `{"version":1}` {
		t.Fatalf("reconciled settings = (%q, %q)", id, document)
	}
	var createdAt string
	if err := reconciled.SQL().QueryRow(
		`SELECT created_at FROM prompt_sets WHERE id = 'p-current'`,
	).Scan(&createdAt); err != nil {
		t.Fatalf("read reconciled prompt set: %v", err)
	}
	if createdAt == "" {
		t.Fatal("current-version prompt timestamp was not backfilled")
	}
}

func TestMigrationUpgradesV8DatabaseToModelInventory(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{"DROP TABLE llm_models", "PRAGMA user_version = 8"} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v8 database: %v", err)
	}
	defer func() { _ = upgraded.Close() }()
	assertTableExists(t, upgraded.SQL(), "llm_models")
	if _, err := upgraded.SQL().Exec(`
		INSERT INTO llm_models(
			id, display_name, provider, source, size_bytes, sha256, model_path,
			imported_at, updated_at
		) VALUES('m1', 'Model', 'llama_cpp', 'ollama', 10, 'abc', 'model.gguf', 'now', 'now')
	`); err != nil {
		t.Fatalf("model inventory is not writable: %v", err)
	}
}

func TestMigrationUpgradesV9ToSettingsRecoveryHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := db.Path()
	if _, err := db.SQL().Exec(`
		INSERT INTO settings(id, document, updated_at)
		VALUES('current', '{"version":1}', '2026-07-18T00:00:00Z')
	`); err != nil {
		_ = db.Close()
		t.Fatalf("seed settings: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		"DROP TABLE settings_recoveries",
		"PRAGMA user_version = 9",
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v9 database: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	assertTableExists(t, upgraded.SQL(), "settings_recoveries")
	var document string
	if err := upgraded.SQL().QueryRow(`SELECT document FROM settings WHERE id = 'current'`).Scan(&document); err != nil {
		t.Fatalf("read settings after v10 migration: %v", err)
	}
	if document != `{"version":1}` {
		t.Fatalf("settings document changed across v10 migration: %q", document)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("table %s count = %d, want 1", table, count)
	}
}
