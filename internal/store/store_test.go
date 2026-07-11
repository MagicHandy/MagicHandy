package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
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

	for _, table := range []string{"patterns", "programs", "pattern_feedback"} {
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
	for _, table := range []string{"messages", "client_cursors", "patterns", "programs", "pattern_feedback"} {
		assertTableExists(t, db.SQL(), table)
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
