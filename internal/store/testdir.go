package store

import "testing"

// TestDir returns a temporary data directory and closes datastore handles before
// the directory is removed (required on Windows so magichandy.db is not locked).
func TestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(CloseAll)
	return dir
}
