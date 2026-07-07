package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AppState stores LSO-compatible runtime selections.
type AppState struct {
	ActivePersonaID string `json:"active_persona_id"`
	OperationMode   string `json:"operation_mode"`
}

// LoadAppState returns the persisted runtime persona and mode selections.
func (db *DB) LoadAppState() (AppState, error) {
	var activePersonaID, operationMode string
	err := db.sql.QueryRow(
		`SELECT active_persona_id, operation_mode FROM app_state WHERE id = 1`,
	).Scan(&activePersonaID, &operationMode)
	if err == sql.ErrNoRows {
		return AppState{OperationMode: "hybrid"}, nil
	}
	if err != nil {
		return AppState{}, fmt.Errorf("load app state: %w", err)
	}
	return AppState{
		ActivePersonaID: strings.TrimSpace(activePersonaID),
		OperationMode:   normalizeOperationMode(operationMode),
	}, nil
}

// SaveAppState persists runtime persona and mode selections.
func (db *DB) SaveAppState(state AppState) (AppState, error) {
	state.ActivePersonaID = strings.TrimSpace(state.ActivePersonaID)
	state.OperationMode = normalizeOperationMode(state.OperationMode)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := db.withWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO app_state (id, active_persona_id, operation_mode, updated_at)
			 VALUES (1, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   active_persona_id = excluded.active_persona_id,
			   operation_mode = excluded.operation_mode,
			   updated_at = excluded.updated_at`,
			state.ActivePersonaID, state.OperationMode, now,
		)
		if err != nil {
			return fmt.Errorf("save app state: %w", err)
		}
		return nil
	})
	if err != nil {
		return AppState{}, err
	}
	return state, nil
}

// SetActivePersonaID updates the active persona id in app state.
func (db *DB) SetActivePersonaID(personaID string) error {
	state, err := db.LoadAppState()
	if err != nil {
		return err
	}
	state.ActivePersonaID = strings.TrimSpace(personaID)
	_, err = db.SaveAppState(state)
	return err
}

func normalizeOperationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "manual", "auto", "hybrid":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "hybrid"
	}
}
