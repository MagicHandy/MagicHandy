package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PersonaRow is one persisted persona profile.
type PersonaRow struct {
	ID             string
	Name           string
	Description    sql.NullString
	SystemPrompt   string
	ToneJSON       sql.NullString
	MoodJSON       sql.NullString
	BoundariesJSON sql.NullString
	MotionBiasJSON sql.NullString
	CreatedAt      sql.NullString
	UpdatedAt      sql.NullString
}

// PersonaPayload is the API-facing persona document.
type PersonaPayload struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Description    *string     `json:"description"`
	SystemPrompt   string      `json:"system_prompt"`
	ToneJSON       *string     `json:"tone_json,omitempty"`
	MoodJSON       *string     `json:"mood_json,omitempty"`
	BoundariesJSON *string     `json:"boundaries_json,omitempty"`
	MotionBiasJSON *string     `json:"motion_bias_json,omitempty"`
	Tone           interface{} `json:"tone,omitempty"`
	Mood           interface{} `json:"mood,omitempty"`
	Boundaries     interface{} `json:"boundaries,omitempty"`
	MotionBias     interface{} `json:"motion_bias,omitempty"`
	CreatedAt      *string     `json:"created_at,omitempty"`
	UpdatedAt      *string     `json:"updated_at,omitempty"`
}

// PersonaWrite is the editable persona fields accepted by SavePersona.
type PersonaWrite struct {
	Name           string
	Description    *string
	SystemPrompt   string
	ToneJSON       *string
	MoodJSON       *string
	BoundariesJSON *string
	MotionBiasJSON *string
}

// ErrPersonaNotFound is returned when a persona id does not exist.
var ErrPersonaNotFound = errors.New("persona not found")

// CountPersonas returns the number of persisted personas.
func (db *DB) CountPersonas() (int, error) {
	var count int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM personas`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count personas: %w", err)
	}
	return count, nil
}

// ListPersonas returns all persona rows ordered by name.
func (db *DB) ListPersonas() ([]PersonaRow, error) {
	rows, err := db.sql.Query(`
		SELECT id, name, description, system_prompt, tone_json, mood_json,
		       boundaries_json, motion_bias_json, created_at, updated_at
		FROM personas
		ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list personas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PersonaRow, 0)
	for rows.Next() {
		row, err := scanPersonaRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate personas: %w", err)
	}
	return out, nil
}

// GetPersona returns one persona row by id.
func (db *DB) GetPersona(id string) (PersonaRow, error) {
	row := db.sql.QueryRow(`
		SELECT id, name, description, system_prompt, tone_json, mood_json,
		       boundaries_json, motion_bias_json, created_at, updated_at
		FROM personas WHERE id = ?`, strings.TrimSpace(id))
	persona, err := scanPersonaRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PersonaRow{}, ErrPersonaNotFound
	}
	return persona, err
}

// SavePersona inserts or updates one persona row.
func (db *DB) SavePersona(id string, write PersonaWrite) (PersonaRow, error) {
	name := strings.TrimSpace(write.Name)
	systemPrompt := strings.TrimSpace(write.SystemPrompt)
	if name == "" {
		return PersonaRow{}, errors.New("persona name is required")
	}
	if systemPrompt == "" {
		return PersonaRow{}, errors.New("persona system_prompt is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(id) == "" {
		id = newPersonaID()
	}

	description := nullString(write.Description)
	toneJSON := nullString(write.ToneJSON)
	moodJSON := nullString(write.MoodJSON)
	boundariesJSON := nullString(write.BoundariesJSON)
	motionBiasJSON := nullString(write.MotionBiasJSON)

	err := db.withWrite(func(tx *sql.Tx) error {
		var existingCreated sql.NullString
		err := tx.QueryRow(`SELECT created_at FROM personas WHERE id = ?`, id).Scan(&existingCreated)
		createdAt := now
		if err == nil && existingCreated.Valid {
			createdAt = existingCreated.String
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup persona %q: %w", id, err)
		}

		_, err = tx.Exec(`
			INSERT INTO personas (
				id, name, description, system_prompt, tone_json, mood_json,
				boundaries_json, motion_bias_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				system_prompt = excluded.system_prompt,
				tone_json = excluded.tone_json,
				mood_json = excluded.mood_json,
				boundaries_json = excluded.boundaries_json,
				motion_bias_json = excluded.motion_bias_json,
				updated_at = excluded.updated_at`,
			id, name, description, systemPrompt, toneJSON, moodJSON,
			boundariesJSON, motionBiasJSON, createdAt, now,
		)
		if err != nil {
			return fmt.Errorf("save persona %q: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return PersonaRow{}, err
	}
	return db.GetPersona(id)
}

// DeletePersona removes one persona row by id.
func (db *DB) DeletePersona(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrPersonaNotFound
	}
	return db.withWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM personas WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete persona %q: %w", id, err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrPersonaNotFound
		}
		return nil
	})
}

// PersonaPayloadFromRow maps a DB row to the API persona document.
func PersonaPayloadFromRow(row PersonaRow) PersonaPayload {
	payload := PersonaPayload{
		ID:           row.ID,
		Name:         row.Name,
		SystemPrompt: row.SystemPrompt,
	}
	if row.Description.Valid {
		payload.Description = &row.Description.String
	}
	if row.ToneJSON.Valid {
		payload.ToneJSON = &row.ToneJSON.String
		payload.Tone = parseJSONValue(row.ToneJSON.String)
	}
	if row.MoodJSON.Valid {
		payload.MoodJSON = &row.MoodJSON.String
		payload.Mood = parseJSONValue(row.MoodJSON.String)
	}
	if row.BoundariesJSON.Valid {
		payload.BoundariesJSON = &row.BoundariesJSON.String
		payload.Boundaries = parseJSONValue(row.BoundariesJSON.String)
	}
	if row.MotionBiasJSON.Valid {
		payload.MotionBiasJSON = &row.MotionBiasJSON.String
		payload.MotionBias = parseJSONValue(row.MotionBiasJSON.String)
	}
	if row.CreatedAt.Valid {
		payload.CreatedAt = &row.CreatedAt.String
	}
	if row.UpdatedAt.Valid {
		payload.UpdatedAt = &row.UpdatedAt.String
	}
	return payload
}

func scanPersonaRow(scanner interface {
	Scan(dest ...any) error
}) (PersonaRow, error) {
	var row PersonaRow
	err := scanner.Scan(
		&row.ID, &row.Name, &row.Description, &row.SystemPrompt,
		&row.ToneJSON, &row.MoodJSON, &row.BoundariesJSON, &row.MotionBiasJSON,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return PersonaRow{}, err
	}
	return row, nil
}

func parseJSONValue(raw string) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func newPersonaID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("persona_%d", time.Now().UnixNano())
	}
	return "persona_" + hex.EncodeToString(buf)
}
