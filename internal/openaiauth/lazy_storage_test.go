package openaiauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalOnlyStartupAndStatusLeaveCloudStorageDormant(t *testing.T) {
	directory := t.TempDir()
	m, err := Open(Options{DataDir: directory, LazyStorage: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	status := m.Status(t.Context())
	if status.State != "disconnected" || len(status.Profiles) != 0 || m.storageReady {
		t.Fatal("empty cloud status initialized credential storage")
	}
	if _, err := os.Stat(filepath.Join(directory, "openai-private")); !os.IsNotExist(err) {
		t.Fatal("local-only startup wrote cloud files")
	}
	if err := m.SetAPIKey(t.Context(), "fixture-key"); err != nil {
		t.Fatal(err)
	}
	if !m.storageReady || !m.Status(t.Context()).DecisionsKeySet {
		t.Fatal("explicit cloud use did not initialize protected storage")
	}
	m.Close()
	next, err := Open(Options{DataDir: directory, LazyStorage: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(next.Close)
	if !next.Status(t.Context()).DecisionsKeySet {
		t.Fatal("lazy reopening lost saved credentials")
	}
}
