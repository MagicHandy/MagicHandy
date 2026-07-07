package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	settingsJSONFile   = "settings.json"
	memoriesJSONFile   = "memories.json"
	promptSetsJSONFile = "prompt_sets.json"
	migratedSuffix     = ".migrated"
)

type legacyMemoriesFile struct {
	Enabled  *bool       `json:"enabled"`
	Memories []MemoryRow `json:"memories"`
}

type legacyPromptSetsFile struct {
	Sets []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		System string `json:"system"`
	} `json:"sets"`
}

func importLegacyJSON(dataDir string, db *DB) (ImportResult, error) {
	var result ImportResult
	var err error
	if result.SettingsImported, err = importSettingsJSON(db, dataDir); err != nil {
		return ImportResult{}, err
	}
	if result.MemoriesImported, err = importMemoriesJSON(db, dataDir); err != nil {
		return ImportResult{}, err
	}
	if result.PromptSetsImported, err = importPromptSetsJSON(db, dataDir); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func importSettingsJSON(db *DB, dataDir string) (bool, error) {
	path := filepath.Join(dataDir, settingsJSONFile)
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, nil
	}
	if err := db.withWrite(func(tx *sql.Tx) error {
		return importSettingsDocument(tx, data)
	}); err != nil {
		return false, err
	}
	if err := renameMigrated(path); err != nil {
		return false, err
	}
	return true, nil
}

func importMemoriesJSON(db *DB, dataDir string) (bool, error) {
	path := filepath.Join(dataDir, memoriesJSONFile)
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, nil
	}
	var file legacyMemoriesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return false, nil
	}
	enabled := true
	if file.Enabled != nil {
		enabled = *file.Enabled
	}
	if err := db.withWrite(func(tx *sql.Tx) error {
		return importMemoriesRows(tx, enabled, file.Memories)
	}); err != nil {
		return false, err
	}
	if err := renameMigrated(path); err != nil {
		return false, err
	}
	return true, nil
}

func importPromptSetsJSON(db *DB, dataDir string) (bool, error) {
	path := filepath.Join(dataDir, promptSetsJSONFile)
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, nil
	}
	var file legacyPromptSetsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return false, nil
	}
	sets := make([]PromptSetRow, 0, len(file.Sets))
	for _, set := range file.Sets {
		sets = append(sets, PromptSetRow{ID: set.ID, Name: set.Name, System: set.System})
	}
	if err := db.withWrite(func(tx *sql.Tx) error {
		return importPromptSetRows(tx, sets)
	}); err != nil {
		return false, err
	}
	if err := renameMigrated(path); err != nil {
		return false, err
	}
	return true, nil
}

func importSettingsDocument(tx *sql.Tx, document []byte) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.Exec(
		`INSERT INTO settings (id, document, updated_at) VALUES (1, ?, ?)`,
		string(document), now,
	)
	if err != nil {
		return fmt.Errorf("import settings document: %w", err)
	}
	return nil
}

func importMemoriesRows(tx *sql.Tx, enabled bool, memories []MemoryRow) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	if _, err := tx.Exec(`INSERT INTO memory_meta (id, enabled) VALUES (1, ?)`, enabledInt); err != nil {
		return fmt.Errorf("import memory switch: %w", err)
	}
	for _, item := range memories {
		itemEnabled := 0
		if item.Enabled {
			itemEnabled = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO memories (id, text, enabled, created_at) VALUES (?, ?, ?, ?)`,
			item.ID, item.Text, itemEnabled, item.CreatedAt,
		); err != nil {
			return fmt.Errorf("import memory %q: %w", item.ID, err)
		}
	}
	return nil
}

func importPromptSetRows(tx *sql.Tx, sets []PromptSetRow) error {
	for _, set := range sets {
		if _, err := tx.Exec(
			`INSERT INTO prompt_sets (id, name, system) VALUES (?, ?, ?)`,
			set.ID, set.Name, set.System,
		); err != nil {
			return fmt.Errorf("import prompt set %q: %w", set.ID, err)
		}
	}
	return nil
}

func renameMigrated(path string) error {
	if err := os.Rename(path, path+migratedSuffix); err != nil {
		return fmt.Errorf("rename %s to migrated: %w", filepath.Base(path), err)
	}
	return nil
}
