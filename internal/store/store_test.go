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

func TestRockfireTablesExistAfterOpen(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	for _, table := range []string{"personas", "ui_preferences", "app_state", "funscript_files"} {
		var name string
		err := db.SQL().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing: %v", table, err)
		}
	}
}
