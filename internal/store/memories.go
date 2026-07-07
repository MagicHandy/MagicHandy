package store

import (
	"database/sql"
	"fmt"
)

// MemoryRow is one durable memory record.
type MemoryRow struct {
	ID        string
	Text      string
	Enabled   bool
	CreatedAt string
}

// LoadMemories returns the global injection switch and memory rows. The third
// return value is false when no memory metadata row exists yet.
func (db *DB) LoadMemories() (bool, []MemoryRow, bool, error) {
	var enabledInt int
	err := db.sql.QueryRow(`SELECT enabled FROM memory_meta WHERE id = 1`).Scan(&enabledInt)
	if err == sql.ErrNoRows {
		return true, nil, false, nil
	}
	if err != nil {
		return true, nil, false, fmt.Errorf("load memory switch: %w", err)
	}

	rows, err := db.sql.Query(`SELECT id, text, enabled, created_at FROM memories ORDER BY rowid`)
	if err != nil {
		return enabledInt == 1, nil, true, fmt.Errorf("load memories: %w", err)
	}
	defer rows.Close()

	memories := make([]MemoryRow, 0)
	for rows.Next() {
		var item MemoryRow
		var enabled int
		if err := rows.Scan(&item.ID, &item.Text, &enabled, &item.CreatedAt); err != nil {
			return enabledInt == 1, nil, true, fmt.Errorf("scan memory: %w", err)
		}
		item.Enabled = enabled == 1
		memories = append(memories, item)
	}
	if err := rows.Err(); err != nil {
		return enabledInt == 1, nil, true, fmt.Errorf("iterate memories: %w", err)
	}
	return enabledInt == 1, memories, true, nil
}

// SaveMemories replaces the global switch and all memory rows atomically.
func (db *DB) SaveMemories(enabled bool, memories []MemoryRow) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	return db.withWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO memory_meta (id, enabled) VALUES (1, ?)
			 ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled`,
			enabledInt,
		); err != nil {
			return fmt.Errorf("save memory switch: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM memories`); err != nil {
			return fmt.Errorf("clear memories: %w", err)
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
				return fmt.Errorf("insert memory %q: %w", item.ID, err)
			}
		}
		return nil
	})
}
