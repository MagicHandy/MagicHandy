package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// Store owns the process-local settings snapshot and durable settings file.
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	path     string
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

	store := &Store{
		dataDir: absDir,
		path:    filepath.Join(absDir, settingsFileName),
	}
	store.settings, store.status = loadSettingsFile(store.path, store.dataDir)
	return store, nil
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
	if err := writeSettingsFile(s.path, next); err != nil {
		return Settings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

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

// Path returns the durable settings file path.
func (s *Store) Path() string {
	return s.path
}

func loadSettingsFile(path string, dataDir string) (Settings, LoadStatus) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := LoadStatus{
		DataDir:      dataDir,
		SettingsPath: path,
		LoadedAt:     now,
	}

	data, err := os.ReadFile(path) // #nosec G304 -- path is the resolved app settings file.
	if err != nil {
		status.Source = loadSourceDefault
		status.UsingDefaults = true
		if !os.IsNotExist(err) {
			status.Source = loadSourceRecover
			status.Recovered = true
			status.Message = "settings file could not be read; defaults are active"
		}
		return DefaultSettings(), status
	}

	settings, migrated, err := loadSettingsFromBytes(data)
	if err != nil {
		status.Source = loadSourceRecover
		status.UsingDefaults = true
		status.Recovered = true
		status.Message = "settings file could not be parsed; defaults are active"
		return DefaultSettings(), status
	}

	status.Source = loadSourceFile
	status.Migrated = migrated
	return settings, status
}
