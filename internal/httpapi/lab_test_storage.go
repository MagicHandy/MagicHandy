package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const labTestRunsKey = "labs.test_runs.v1"
const maximumLabTestRuns = 20
const maximumLabTestBytes = 16 << 20

var errLabTestNotFound = errors.New("test sequence was not found")
var errLabTestChanged = errors.New("test progress changed; reload the sequence before saving")

func decodeLabTestRuns(document string, readErr error) ([]labTestRun, error) {
	if errors.Is(readErr, sql.ErrNoRows) {
		return []labTestRun{}, nil
	}
	if readErr != nil {
		return nil, errors.New("test sequences could not be loaded")
	}
	if len(document) > maximumLabTestBytes {
		return nil, errors.New("test sequences exceed their storage limit")
	}
	var runs []labTestRun
	if err := json.Unmarshal([]byte(document), &runs); err != nil {
		return nil, errors.New("test sequences could not be decoded")
	}
	if runs == nil {
		runs = []labTestRun{}
	}
	return runs, nil
}

func (s *Server) readLabTestRuns(ctx context.Context) ([]labTestRun, error) {
	var document string
	err := s.store.Datastore().SQL().QueryRowContext(ctx, "SELECT value FROM app_kv WHERE key = ?", labTestRunsKey).Scan(&document)
	return decodeLabTestRuns(document, err)
}

func (s *Server) updateLabTestRuns(ctx context.Context, change func([]labTestRun) ([]labTestRun, error)) ([]labTestRun, error) {
	var updated []labTestRun
	err := s.store.Datastore().WithTx(ctx, func(tx *sql.Tx) error {
		var document string
		readErr := tx.QueryRowContext(ctx, "SELECT value FROM app_kv WHERE key = ?", labTestRunsKey).Scan(&document)
		runs, err := decodeLabTestRuns(document, readErr)
		if err != nil {
			return err
		}
		updated, err = change(runs)
		if err != nil {
			return err
		}
		data, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if len(data) > maximumLabTestBytes {
			return errors.New("test sequences are full; export and remove older sequences")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app_kv(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, labTestRunsKey, string(data), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	return updated, err
}
