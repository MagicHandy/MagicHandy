package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const labObservationsKey = "labs.observations.v1"
const maximumLabObservations = 200
const maximumLabObservationBytes = 4 << 20

var errObservationNotFound = errors.New("saved observation was not found")

// Observations are durable review evidence, never implicit model context or
// motion preferences. A saved reply includes its score and original output so
// it remains meaningful after the temporary conversation is reset or trimmed.
type labObservation struct {
	ID        string                `json:"id"`
	CreatedAt string                `json:"created_at"`
	Text      string                `json:"text"`
	Source    string                `json:"source"`
	Label     string                `json:"label"`
	Method    string                `json:"method"`
	Spec      motion.FlowSpec       `json:"spec"`
	Settings  config.MotionSettings `json:"settings"`
	Trial     *chat.LLMLabTrial     `json:"trial,omitempty"`
}

type observationRequest struct {
	Text        string           `json:"text"`
	Source      string           `json:"source"`
	Method      string           `json:"method"`
	Spec        *motion.FlowSpec `json:"spec,omitempty"`
	SettingsKey string           `json:"settings_key"`
	Revision    uint64           `json:"revision"`
	TurnIndex   *int             `json:"turn_index,omitempty"`
}

func (s *Server) handleLabObservations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.readLabObservations(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("saved observations could not be loaded"))
		return
	}
	s.writeLabObservations(w, rows)
}

func (s *Server) writeLabObservations(w http.ResponseWriter, rows []labObservation) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"observations": rows, "storage_path": s.store.Datastore().Path(), "capacity": maximumLabObservations})
}

func (s *Server) handleSaveLabObservation(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body observationRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	observation, err := s.prepareLabObservation(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	rows, err := s.updateLabObservations(r.Context(), func(rows []labObservation) ([]labObservation, error) {
		if len(rows) >= maximumLabObservations {
			return nil, errors.New("saved observations are full; export and remove an observation before saving another")
		}
		return append([]labObservation{observation}, rows...), nil
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	s.writeLabObservations(w, rows)
}

func (s *Server) prepareLabObservation(body observationRequest) (labObservation, error) {
	text := strings.TrimSpace(body.Text)
	if text == "" || utf8.RuneCountInString(text) > 2000 {
		return labObservation{}, errors.New("write an observation of 1 to 2000 characters")
	}
	settings, _ := s.store.Snapshot()
	row := labObservation{ID: rand.Text(), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Text: text, Source: body.Source, Settings: settings.Motion}
	switch body.Source {
	case "motion":
		if body.SettingsKey != motion.LabSettingsKey(settings.Motion) {
			return labObservation{}, errors.New("saved limits changed; refresh the preview before recording an observation")
		}
		if body.Spec == nil || body.Method != "flow" && body.Method != "creative" && body.Method != "anchored" {
			return labObservation{}, errors.New("select the motion preview this observation describes")
		}
		if err := body.Spec.Validate(settings.Motion); err != nil {
			return labObservation{}, err
		}
		row.Spec, row.Method, row.Label = *body.Spec, body.Method, "Motion preview"
	case "llm":
		state := s.labState()
		if state.Revision != body.Revision || body.TurnIndex == nil || *body.TurnIndex < 0 || *body.TurnIndex >= len(state.Turns) {
			return labObservation{}, errors.New("the lab conversation changed; select the reply again")
		}
		trial := state.Turns[*body.TurnIndex]
		row.Spec, row.Trial, row.Method, row.Label = trial.After, &trial, trial.Method, "LLM reply"
		row.Settings = trial.Limits
		if trial.RecipeName != "" {
			row.Label = trial.RecipeName
		}
	default:
		return labObservation{}, errors.New("select a motion preview or LLM reply")
	}
	return row, nil
}

func (s *Server) handleDeleteLabObservation(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	rows, err := s.updateLabObservations(r.Context(), func(rows []labObservation) ([]labObservation, error) {
		for index, row := range rows {
			if row.ID == r.PathValue("id") {
				return append(rows[:index], rows[index+1:]...), nil
			}
		}
		return nil, errObservationNotFound
	})
	if err != nil {
		if errors.Is(err, errObservationNotFound) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusServiceUnavailable, errors.New("saved observation could not be deleted"))
		}
		return
	}
	s.writeLabObservations(w, rows)
}

func (s *Server) readLabObservations(ctx context.Context) ([]labObservation, error) {
	var document string
	err := s.store.Datastore().SQL().QueryRowContext(ctx, "SELECT value FROM app_kv WHERE key = ?", labObservationsKey).Scan(&document)
	return decodeLabObservations(document, err)
}

func decodeLabObservations(document string, readErr error) ([]labObservation, error) {
	if errors.Is(readErr, sql.ErrNoRows) {
		return []labObservation{}, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	if len(document) > maximumLabObservationBytes {
		return nil, errors.New("saved observations exceed their storage limit")
	}
	var rows []labObservation
	if err := json.Unmarshal([]byte(document), &rows); err != nil {
		return nil, errors.New("saved observations could not be decoded")
	}
	if rows == nil {
		rows = []labObservation{}
	}
	return rows, nil
}

func (s *Server) updateLabObservations(ctx context.Context, change func([]labObservation) ([]labObservation, error)) ([]labObservation, error) {
	var updated []labObservation
	err := s.store.Datastore().WithTx(ctx, func(tx *sql.Tx) error {
		var document string
		readErr := tx.QueryRowContext(ctx, "SELECT value FROM app_kv WHERE key = ?", labObservationsKey).Scan(&document)
		rows, err := decodeLabObservations(document, readErr)
		if err != nil {
			return err
		}
		updated, err = change(rows)
		if err != nil {
			return err
		}
		data, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if len(data) > maximumLabObservationBytes {
			return errors.New("saved observations exceed their storage limit; export and remove older records")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app_kv(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, labObservationsKey, string(data), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	return updated, err
}
