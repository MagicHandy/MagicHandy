package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	settingsFileName = "settings.json"

	loadSourceDefault = "default"
	loadSourceFile    = "file"
	loadSourceRecover = "recovered_default"
)

// LoadStatus describes how settings were loaded without exposing setting values.
type LoadStatus struct {
	DataDir       string `json:"data_dir"`
	SettingsPath  string `json:"settings_path"`
	Source        string `json:"source"`
	UsingDefaults bool   `json:"using_defaults"`
	Recovered     bool   `json:"recovered"`
	Migrated      bool   `json:"migrated"`
	Message       string `json:"message,omitempty"`
	LoadedAt      string `json:"loaded_at"`
}

// Store owns the process-local settings snapshot and durable settings document.
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	path     string
	db       *store.DB
	settings Settings
	status   LoadStatus
}

// ResolveDataDir returns the configured app data directory.
func ResolveDataDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if fromEnv := os.Getenv("MAGICHANDY_DATA_DIR"); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "MagicHandy"), nil
}

// OpenStore loads settings from dataDir and falls back to defaults safely.
func OpenStore(dataDir string) (*Store, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}

	db, err := store.Open(absDir)
	if err != nil {
		return nil, err
	}

	s := &Store{
		dataDir: absDir,
		path:    db.Path(),
		db:      db,
	}
	s.settings, s.status = loadSettings(db)
	return s, nil
}

// Snapshot returns the current settings and load status.
func (s *Store) Snapshot() (Settings, LoadStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.settings, s.status
}

// PublicSnapshot returns the current redacted settings and load status.
func (s *Store) PublicSnapshot() (PublicSettings, LoadStatus) {
	settings, status := s.Snapshot()
	return settings.Public(), status
}

// Save validates, writes, and publishes the next settings snapshot.
func (s *Store) Save(next Settings) (Settings, error) {
	next, err := NormalizeSettings(next)
	if err != nil {
		return Settings{}, err
	}

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.db.SaveSettingsDocument(data); err != nil {
		return Settings{}, err
	}
	s.settings = next
	s.status = LoadStatus{
		DataDir:      s.dataDir,
		SettingsPath: s.path,
		Source:       loadSourceFile,
		LoadedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	return next, nil
}

// DataDir returns the resolved settings directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// Path returns the durable datastore path.
func (s *Store) Path() string {
	return s.path
}

// DB exposes the embedded SQLite datastore for UI and library features.
func (s *Store) DB() *store.DB {
	return s.db
}

// Close releases the embedded SQLite datastore.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func loadSettings(db *store.DB) (Settings, LoadStatus) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := LoadStatus{
		DataDir:      db.DataDir(),
		SettingsPath: db.Path(),
		LoadedAt:     now,
	}
	if db.ImportResult().SettingsImported {
		status.Migrated = true
	}

	document, found, err := db.LoadSettingsDocument()
	if err != nil {
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "settings could not be read; defaults are active"
		return DefaultSettings(), status
	}
	if !found {
		if recovered, message := recoverLegacySettings(db.DataDir()); recovered {
			status.Source = loadSourceRecover
			status.UsingDefaults = true
			status.Recovered = true
			status.Message = message
		} else {
			status.Source = loadSourceDefault
			status.UsingDefaults = true
		}
		return DefaultSettings(), status
	}

	settings, migrated, err := loadSettingsFromBytes(document)
	if err != nil {
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "settings could not be parsed; defaults are active"
		return DefaultSettings(), status
	}

	status.Source = loadSourceFile
	status.Migrated = status.Migrated || migrated
	return settings, status
}

func recoverLegacySettings(dataDir string) (bool, string) {
	path := filepath.Join(dataDir, settingsFileName)
	if _, err := os.Stat(path); err != nil {
		return false, ""
	}
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app settings file.
	if err != nil {
		return true, "settings file could not be read; defaults are active"
	}
	if _, _, err := loadSettingsFromBytes(data); err != nil {
		return true, "settings file could not be parsed; defaults are active"
	}
	return false, ""
}
