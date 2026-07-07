package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesDatabaseAndRunsMigrations(t *testing.T) {
	dir := TestDir(t)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	version, err := schemaUserVersion(db.sql)
	if err != nil {
		t.Fatalf("schemaUserVersion: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	if _, err := os.Stat(filepath.Join(dir, DBFileName)); err != nil {
		t.Fatalf("database file missing: %v", err)
	}
}

func TestImportRenamesLegacyJSONFiles(t *testing.T) {
	dir := TestDir(t)
	settings := []byte(`{"version":1,"server":{"port":49730}}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, settingsJSONFile), settings, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	enabled := true
	memories, err := json.Marshal(struct {
		Enabled  *bool       `json:"enabled"`
		Memories []MemoryRow `json:"memories"`
	}{
		Enabled:  &enabled,
		Memories: []MemoryRow{{ID: "mem-1", Text: "Remember this.", Enabled: true, CreatedAt: "2026-07-06T00:00:00Z"}},
	})
	if err != nil {
		t.Fatalf("marshal memories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, memoriesJSONFile), memories, 0o600); err != nil {
		t.Fatalf("write memories: %v", err)
	}

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	result := db.ImportResult()
	if !result.SettingsImported || !result.MemoriesImported {
		t.Fatalf("import result = %+v, want settings and memories imported", result)
	}
	if _, err := os.Stat(filepath.Join(dir, settingsJSONFile)); !os.IsNotExist(err) {
		t.Fatal("settings.json was not renamed after import")
	}
	if _, err := os.Stat(filepath.Join(dir, settingsJSONFile+migratedSuffix)); err != nil {
		t.Fatalf("settings.json.migrated missing: %v", err)
	}

	document, found, err := db.LoadSettingsDocument()
	if err != nil || !found {
		t.Fatalf("LoadSettingsDocument: found=%v err=%v", found, err)
	}
	if string(document) != string(settings) {
		t.Fatalf("settings document = %q, want %q", document, settings)
	}
	enabled, rows, found, err := db.LoadMemories()
	if err != nil || !found || !enabled || len(rows) != 1 {
		t.Fatalf("LoadMemories = enabled=%v rows=%+v found=%v err=%v", enabled, rows, found, err)
	}
}

func TestNewerSchemaVersionIsRejected(t *testing.T) {
	dir := TestDir(t)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.sql.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	_ = db.Close()

	if _, err := Open(dir); err == nil {
		t.Fatal("Open with newer schema did not fail")
	}
}

func TestSettingsDocumentRoundTrip(t *testing.T) {
	dir := TestDir(t)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	payload := []byte(`{"version":1}` + "\n")
	if err := db.SaveSettingsDocument(payload); err != nil {
		t.Fatalf("SaveSettingsDocument: %v", err)
	}
	got, found, err := db.LoadSettingsDocument()
	if err != nil {
		t.Fatalf("LoadSettingsDocument: %v", err)
	}
	if !found || string(got) != string(payload) {
		t.Fatalf("document = %q found=%v, want %q", got, found, payload)
	}
}
