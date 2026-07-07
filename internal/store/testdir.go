package store

import (
	"path/filepath"
	"testing"
)

// TestDir returns a temporary data directory and closes datastore handles opened
// under that directory before the directory is removed (required on Windows so
// magichandy.db is not locked).
func TestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { closeAllInDir(dir) })
	return dir
}

func closeAllInDir(dir string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		CloseAll()
		return
	}

	trackMu.Lock()
	defer trackMu.Unlock()
	next := tracked[:0]
	for _, db := range tracked {
		if db.dataDir == absDir {
			_ = db.sql.Close()
			continue
		}
		next = append(next, db)
	}
	tracked = next
}
