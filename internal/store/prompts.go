package store

import (
	"database/sql"
	"fmt"
)

// PromptSetRow is one user prompt set row (built-ins never enter the database).
type PromptSetRow struct {
	ID     string
	Name   string
	System string
}

// LoadPromptSets returns user prompt sets. The second return value is false when
// no rows exist yet.
func (db *DB) LoadPromptSets() ([]PromptSetRow, bool, error) {
	rows, err := db.sql.Query(`SELECT id, name, system FROM prompt_sets ORDER BY name, id`)
	if err != nil {
		return nil, false, fmt.Errorf("load prompt sets: %w", err)
	}
	defer rows.Close()

	sets := make([]PromptSetRow, 0)
	for rows.Next() {
		var set PromptSetRow
		if err := rows.Scan(&set.ID, &set.Name, &set.System); err != nil {
			return nil, false, fmt.Errorf("scan prompt set: %w", err)
		}
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate prompt sets: %w", err)
	}
	return sets, len(sets) > 0, nil
}

// SavePromptSets replaces all user prompt sets atomically.
func (db *DB) SavePromptSets(sets []PromptSetRow) error {
	return db.withWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM prompt_sets`); err != nil {
			return fmt.Errorf("clear prompt sets: %w", err)
		}
		for _, set := range sets {
			if _, err := tx.Exec(
				`INSERT INTO prompt_sets (id, name, system) VALUES (?, ?, ?)`,
				set.ID, set.Name, set.System,
			); err != nil {
				return fmt.Errorf("insert prompt set %q: %w", set.ID, err)
			}
		}
		return nil
	})
}
