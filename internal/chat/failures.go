package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// FailureRetention bounds all sessions together; failed attempts are never
// conversation messages and cannot reach model history, mood or speech.
const FailureRetention = 24

// RequestFailure contains non-secret provenance. Content is fetched explicitly.
type RequestFailure struct {
	ID            int64              `json:"id"`
	UserSeq       int64              `json:"user_seq,omitempty"`
	CreatedAt     string             `json:"created_at"`
	Error         string             `json:"error"`
	Kind          string             `json:"kind,omitempty"`
	RepairOutcome string             `json:"repair_outcome,omitempty"`
	MotionMode    string             `json:"motion_mode"`
	ReasoningMode string             `json:"reasoning_mode"`
	MaxTokens     int                `json:"max_tokens"`
	Version       string             `json:"app_version"`
	Commit        string             `json:"app_commit"`
	Diagnostics   MessageDiagnostics `json:"diagnostics"`
	Details       *FailureContent    `json:"content,omitempty"`
}

// FailureContent is redacted before storage and omitted from default reports.
type FailureContent struct {
	Raw          string `json:"raw_response"`
	RepairRaw    string `json:"repair_response,omitempty"`
	InitialError string `json:"initial_error,omitempty"`
	Prompt       string `json:"system_prompt"`
	Schema       string `json:"response_schema"`
	Truncated    bool   `json:"truncated"`
}

// SaveFailure retains only a bounded diagnostic record; it publishes no reply.
func (l *MessageLog) SaveFailure(ctx context.Context, sessionID string, failure RequestFailure) (int64, error) {
	content := failure.Details
	failure.Details, failure.ID = nil, 0
	failure.CreatedAt = nowUTC()
	metadata, err := json.Marshal(failure)
	if err != nil || len(metadata) > 8192 {
		return 0, errors.New("failure metadata exceeds its limit")
	}
	details, err := json.Marshal(content)
	if err != nil || len(details) > 262144 {
		return 0, errors.New("failure content exceeds its limit")
	}
	var id int64
	err = l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO chat_failures(session_id, metadata, details) VALUES(?,?,?)`, sessionID, string(metadata), string(details))
		if insertErr != nil {
			return insertErr
		}
		id, insertErr = result.LastInsertId()
		if insertErr != nil {
			return insertErr
		}
		_, pruneErr := tx.ExecContext(ctx, `DELETE FROM chat_failures WHERE seq NOT IN (SELECT seq FROM chat_failures ORDER BY seq DESC LIMIT ?)`, FailureRetention)
		return pruneErr
	})
	return id, err
}

// SetFailureRepairOutcome amends only the non-secret cancellation outcome of
// an already retained attempt. Deletion/retention may have removed it meanwhile.
func (l *MessageLog) SetFailureRepairOutcome(ctx context.Context, sessionID string, id int64, outcome string) error {
	if outcome != "stopped" && outcome != "canceled" && outcome != "stale" {
		return errors.New("invalid repair cancellation outcome")
	}
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var metadata string
		if err := tx.QueryRowContext(ctx, `SELECT metadata FROM chat_failures WHERE session_id=? AND seq=?`, sessionID, id).Scan(&metadata); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		var failure RequestFailure
		if err := json.Unmarshal([]byte(metadata), &failure); err != nil {
			return err
		}
		if failure.RepairOutcome == outcome {
			return nil
		}
		failure.RepairOutcome = outcome
		encoded, err := json.Marshal(failure)
		if err != nil {
			return err
		}
		if len(encoded) > 8192 {
			return errors.New("failure metadata exceeds its limit")
		}
		_, err = tx.ExecContext(ctx, `UPDATE chat_failures SET metadata=? WHERE session_id=? AND seq=?`, string(encoded), sessionID, id)
		return err
	})
}

// Failures reads a bounded metadata list for one conversation, newest first.
func (l *MessageLog) Failures(ctx context.Context, sessionID string) ([]RequestFailure, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT seq, metadata FROM chat_failures WHERE session_id = ? ORDER BY seq DESC LIMIT ?`, sessionID, FailureRetention)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]RequestFailure, 0)
	for rows.Next() {
		var id int64
		var document string
		if err := rows.Scan(&id, &document); err != nil {
			return nil, err
		}
		var item RequestFailure
		if err := json.Unmarshal([]byte(document), &item); err != nil {
			return nil, err
		}
		item.ID, item.Details = id, nil
		result = append(result, item)
	}
	return result, rows.Err()
}

// Failure fetches private model content only when explicitly requested.
func (l *MessageLog) Failure(ctx context.Context, sessionID string, id int64, includeContent bool) (RequestFailure, error) {
	column := "'null'"
	if includeContent {
		column = "details"
	}
	var metadata, details string
	err := l.db.SQL().QueryRowContext(ctx, `SELECT metadata, `+column+` FROM chat_failures WHERE session_id = ? AND seq = ?`, sessionID, id).Scan(&metadata, &details)
	if err != nil {
		return RequestFailure{}, err
	}
	var result RequestFailure
	if err = json.Unmarshal([]byte(metadata), &result); err != nil {
		return result, fmt.Errorf("decode failed request: %w", err)
	}
	result.ID = id
	err = json.Unmarshal([]byte(details), &result.Details)
	return result, err
}
