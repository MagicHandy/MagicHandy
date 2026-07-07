package store

import (
	"database/sql"
	"fmt"
	"time"
)

// LoadSettingsDocument returns the stored settings JSON document. The second
// return value is false when no row exists yet.
func (db *DB) LoadSettingsDocument() ([]byte, bool, error) {
	var document string
	err := db.sql.QueryRow(`SELECT document FROM settings WHERE id = 1`).Scan(&document)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load settings document: %w", err)
	}
	return []byte(document), true, nil
}

// SaveSettingsDocument persists the settings JSON document as a single row.
func (db *DB) SaveSettingsDocument(document []byte) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return db.withWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO settings (id, document, updated_at) VALUES (1, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET document = excluded.document, updated_at = excluded.updated_at`,
			string(document), now,
		)
		if err != nil {
			return fmt.Errorf("save settings document: %w", err)
		}
		return nil
	})
}
