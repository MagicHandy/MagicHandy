package httpapi

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// migratedDatastore is an empty datastore at the current schema version, built
// once per test binary. Fixtures copy it into their own data directory, so each
// test still gets a private database, while the schema migrations (most of
// this package's SQLite work, and many times slower under -race) run once
// instead of for every server. internal/store tests cover the migrations.
var migratedDatastore struct {
	once sync.Once
	data []byte
	err  error
}

// openTestStore is config.OpenStore on a fresh temporary data directory that
// starts from the migrated datastore. Legacy import and default settings still
// run for each store, exactly as on a first start.
func openTestStore(t *testing.T) (*config.Store, error) {
	t.Helper()
	migratedDatastore.once.Do(func() {
		dir, err := os.MkdirTemp("", "magichandy-httpapi-schema-")
		if err != nil {
			migratedDatastore.err = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		db, err := dbstore.Open(dir)
		if err != nil {
			migratedDatastore.err = err
			return
		}
		path := db.Path()
		if err := db.Close(); err != nil {
			migratedDatastore.err = err
			return
		}
		migratedDatastore.data, migratedDatastore.err = os.ReadFile(path) // #nosec G304 -- temporary fixture path.
	})
	if migratedDatastore.err != nil {
		return nil, migratedDatastore.err
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dbstore.DatabaseFileName), migratedDatastore.data, 0o600); err != nil {
		return nil, err
	}
	return config.OpenStore(dir)
}
