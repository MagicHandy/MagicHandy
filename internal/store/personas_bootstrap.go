package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/personabuiltin"
)

// SeedClarissaSynsualPersona inserts the built-in Clarissa persona when missing.
func SeedClarissaSynsualPersona(ctx context.Context, tx *sql.Tx) error {
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT id FROM personas WHERE id = ?`, personabuiltin.ClarissaSynsualID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("lookup clarissa persona: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	description := personabuiltin.ClarissaSynsualDescription
	_, err = tx.ExecContext(ctx, `
		INSERT INTO personas (
			id, name, description, system_prompt, tone_json, mood_json,
			boundaries_json, motion_bias_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULL, NULL, NULL, NULL, ?, ?)
	`, personabuiltin.ClarissaSynsualID, personabuiltin.ClarissaSynsualName, description,
		personabuiltin.ClarissaSynsualSystem, now, now)
	if err != nil {
		return fmt.Errorf("seed clarissa persona: %w", err)
	}
	return nil
}
