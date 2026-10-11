package chat

import (
	"context"
	"encoding/json"
)

// SessionFailure keeps failures associated with their conversation without
// reading successful messages or conversation titles into a diagnostic export.
type SessionFailure struct {
	SessionID string `json:"session_id"`
	RequestFailure
}

// FailuresForExport reads the retained failure ring across the shared workspace.
// Callers enforce host-administrator access and explicitly select private content.
func (l *MessageLog) FailuresForExport(ctx context.Context, includeContent bool) ([]SessionFailure, error) {
	column := "'null'"
	if includeContent {
		column = "details"
	}
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT session_id, seq, metadata, `+column+` FROM chat_failures ORDER BY seq DESC LIMIT ?`, FailureRetention)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]SessionFailure, 0)
	for rows.Next() {
		var item SessionFailure
		var id int64
		var metadata, content string
		if err := rows.Scan(&item.SessionID, &id, &metadata, &content); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metadata), &item.RequestFailure); err != nil {
			return nil, err
		}
		item.ID, item.Details = id, nil
		if err := json.Unmarshal([]byte(content), &item.Details); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
