package chat

import (
	"context"
	"fmt"
	"slices"
)

// RecentPromptHistoryContext bounds materialized text as well as rows before
// building a hosted request. Large old imports must not allocate hundreds of
// megabytes merely because a provider can see more messages. A one-byte probe
// distinguishes a full message from an oversized one; partial text is never
// returned to the model. Visible history uses its independent paged reader.
func (l *MessageLog) RecentPromptHistoryContext(ctx context.Context, sessionID string, messageLimit, byteLimit int) ([]LogMessage, int, error) {
	messageLimit = max(1, min(messageLimit, 512))
	byteLimit = max(1, min(byteLimit, 192000))
	return l.promptHistory(ctx, sessionID, messageLimit, byteLimit)
}

// RecentCompactPromptHistoryContext preserves the compact composer’s existing
// row window. It reports omissions from the same read as the dialogue rows.
func (l *MessageLog) RecentCompactPromptHistoryContext(ctx context.Context, sessionID string) ([]LogMessage, int, error) {
	return l.promptHistory(ctx, sessionID, PromptHistoryLimit, 0)
}

func (l *MessageLog) promptHistory(ctx context.Context, sessionID string, messageLimit, byteLimit int) ([]LogMessage, int, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT seq, role,
		CASE WHEN ?=0 THEN content ELSE substr(CAST(content AS BLOB), 1, ?) END, created_at,
		(SELECT COUNT(*) FROM messages WHERE session_id = ? AND committed = 1)
		FROM messages WHERE session_id = ? AND committed = 1 ORDER BY seq DESC LIMIT ?`, byteLimit, byteLimit+1, sessionID, sessionID, messageLimit)
	if err != nil {
		return nil, 0, fmt.Errorf("read bounded prompt history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var history []LogMessage
	remaining := byteLimit
	available := 0
	for rows.Next() {
		var message LogMessage
		var content []byte
		if err := rows.Scan(&message.Seq, &message.Role, &content, &message.CreatedAt, &available); err != nil {
			return nil, 0, fmt.Errorf("scan bounded prompt history: %w", err)
		}
		if byteLimit > 0 {
			if len(content) > remaining {
				break
			}
			remaining -= len(content)
		}
		message.Content = string(content)
		history = append(history, message)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	slices.Reverse(history)
	return history, available - len(history), nil
}
