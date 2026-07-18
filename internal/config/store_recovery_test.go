package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorruptSettingsRecoverToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	settings, status := store.Snapshot()
	if !status.Recovered || !status.UsingDefaults {
		t.Fatalf("status = %+v, want recovered defaults", status)
	}
	if settings.Server.Port != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", settings.Server.Port, DefaultServerPort)
	}
}

func TestCorruptSQLiteRecoveryIsReportedWithPreservedPath(t *testing.T) {
	dir := t.TempDir()
	original := []byte("corrupt SQLite fixture that must be preserved")
	if err := os.WriteFile(filepath.Join(dir, "magichandy.db"), original, 0o600); err != nil {
		t.Fatalf("write corrupt datastore: %v", err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore corrupt datastore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, status := store.Snapshot()
	if !status.Recovered || !status.UsingDefaults || status.Source != loadSourceRecover {
		t.Fatalf("status = %+v, want reported datastore recovery", status)
	}
	if status.DatastoreRecoveredPath == "" || !strings.Contains(status.Message, "preserved") {
		t.Fatalf("recovery status lacks preserved path/message: %+v", status)
	}
	preserved, err := os.ReadFile(filepath.Join(status.DatastoreRecoveredPath, "magichandy.db")) // #nosec G304 -- app-reported temp recovery path.
	if err != nil {
		t.Fatalf("read preserved datastore: %v", err)
	}
	if !bytes.Equal(preserved, original) {
		t.Fatalf("preserved datastore bytes changed: %q", preserved)
	}
	if settings.Server.Port != DefaultServerPort {
		t.Fatalf("recovered server port = %d, want %d", settings.Server.Port, DefaultServerPort)
	}
	settings.Server.Port++
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("save after datastore recovery: %v", err)
	}
	_, savedStatus := store.Snapshot()
	if !savedStatus.Recovered || savedStatus.DatastoreRecoveredPath != status.DatastoreRecoveredPath {
		t.Fatalf("save cleared datastore recovery status: %+v", savedStatus)
	}
}

func TestInvalidSQLiteSettingsArePreservedBeforeDefaults(t *testing.T) {
	tests := []struct {
		name     string
		document string
		message  string
	}{
		{name: "malformed", document: "{broken", message: "invalid settings were preserved"},
		{name: "oversized", document: strings.Repeat("x", maxSettingsDocumentBytes+1), message: "oversized settings were preserved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			seed, err := OpenStore(dir)
			if err != nil {
				t.Fatalf("OpenStore seed: %v", err)
			}
			if _, err := seed.Datastore().SQL().Exec(`
				INSERT INTO settings(id, document, updated_at)
				VALUES('current', ?, 'fixture')
			`, test.document); err != nil {
				_ = seed.Close()
				t.Fatalf("seed invalid settings: %v", err)
			}
			if err := seed.Close(); err != nil {
				t.Fatalf("close seed: %v", err)
			}

			recovered, err := OpenStore(dir)
			if err != nil {
				t.Fatalf("OpenStore invalid settings: %v", err)
			}
			t.Cleanup(func() { _ = recovered.Close() })
			settings, status := recovered.Snapshot()
			if !status.Recovered || !status.UsingDefaults || !strings.Contains(status.Message, test.message) {
				t.Fatalf("status = %+v, want preserved defaults", status)
			}
			if settings.Server.Port != DefaultServerPort {
				t.Fatalf("default port = %d, want %d", settings.Server.Port, DefaultServerPort)
			}
			var preserved string
			if err := recovered.Datastore().SQL().QueryRow(`
				SELECT document FROM settings_recoveries ORDER BY id DESC LIMIT 1
			`).Scan(&preserved); err != nil {
				t.Fatalf("read settings recovery: %v", err)
			}
			if preserved != test.document {
				t.Fatalf("preserved document length = %d, want %d", len(preserved), len(test.document))
			}
			var activeRows int
			if err := recovered.Datastore().SQL().QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&activeRows); err != nil {
				t.Fatalf("count active settings: %v", err)
			}
			if activeRows != 0 {
				t.Fatalf("active invalid settings rows = %d, want 0", activeRows)
			}

			reopened, err := OpenStore(dir)
			if err != nil {
				t.Fatalf("reopen active recovery: %v", err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			_, reopenedStatus := reopened.Snapshot()
			if !reopenedStatus.Recovered || !reopenedStatus.UsingDefaults ||
				!strings.Contains(reopenedStatus.Message, "remains active") {
				t.Fatalf("reopened recovery status = %+v", reopenedStatus)
			}
		})
	}
}

func TestSettingsRecoveryHistoryIsBounded(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for index := 0; index < maxSettingsRecoveries+5; index++ {
		if _, err := store.Datastore().SQL().Exec(`
			INSERT INTO settings(id, document, updated_at)
			VALUES('current', ?, 'fixture')
			ON CONFLICT(id) DO UPDATE SET document = excluded.document
		`, fmt.Sprintf("invalid-%d", index)); err != nil {
			t.Fatalf("seed recovery %d: %v", index, err)
		}
		if err := store.recoverSettingsDocument(context.Background(), "test recovery"); err != nil {
			t.Fatalf("recover settings %d: %v", index, err)
		}
	}
	var count int
	if err := store.Datastore().SQL().QueryRow(`SELECT COUNT(*) FROM settings_recoveries`).Scan(&count); err != nil {
		t.Fatalf("count settings recoveries: %v", err)
	}
	if count != maxSettingsRecoveries {
		t.Fatalf("settings recovery count = %d, want %d", count, maxSettingsRecoveries)
	}
}

func TestSQLiteSettingsMigrationIsDurable(t *testing.T) {
	dir := t.TempDir()
	seed, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore seed: %v", err)
	}
	if _, err := seed.Datastore().SQL().Exec(`
		INSERT INTO settings(id, document, updated_at)
		VALUES('current', '{"version":0}', 'fixture')
	`); err != nil {
		_ = seed.Close()
		t.Fatalf("seed legacy settings document: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed: %v", err)
	}

	migrated, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore legacy document: %v", err)
	}
	_, status := migrated.Snapshot()
	if !status.Migrated {
		_ = migrated.Close()
		t.Fatalf("migration status = %+v", status)
	}
	var document string
	if err := migrated.Datastore().SQL().QueryRow(`SELECT document FROM settings WHERE id = 'current'`).Scan(&document); err != nil {
		_ = migrated.Close()
		t.Fatalf("read migrated document: %v", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(document), &header); err != nil {
		_ = migrated.Close()
		t.Fatalf("decode migrated document: %v", err)
	}
	if header.Version != CurrentSettingsVersion {
		_ = migrated.Close()
		t.Fatalf("persisted settings version = %d, want %d", header.Version, CurrentSettingsVersion)
	}
	if err := migrated.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen migrated document: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	_, reopenedStatus := reopened.Snapshot()
	if reopenedStatus.Migrated {
		t.Fatalf("durable settings migration repeated: %+v", reopenedStatus)
	}
}

func TestSaveRejectsOversizedSettingsBeforeWriting(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	settings.Voice.NeuTTSReferenceCodes = strings.Repeat("1,", maxSettingsDocumentBytes)
	if _, err := store.Save(settings); !errors.Is(err, errSettingsDocumentTooLarge) {
		t.Fatalf("Save oversized settings error = %v, want size-limit error", err)
	}
	var count int
	if err := store.Datastore().SQL().QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&count); err != nil {
		t.Fatalf("count settings after rejected save: %v", err)
	}
	if count != 0 {
		t.Fatalf("oversized save wrote %d settings rows", count)
	}
}

func TestOversizedLegacySettingsRemainUnmodified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, settingsFileName)
	original := bytes.Repeat([]byte("x"), maxSettingsDocumentBytes+1)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write oversized legacy settings: %v", err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, status := store.Snapshot()
	if !status.Recovered || !status.UsingDefaults || !strings.Contains(status.Message, "size limit") {
		t.Fatalf("oversized legacy status = %+v", status)
	}
	preserved, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary legacy path.
	if err != nil {
		t.Fatalf("read preserved legacy settings: %v", err)
	}
	if !bytes.Equal(preserved, original) {
		t.Fatal("oversized legacy settings changed")
	}
}
