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
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT seq, role,
		substr(CAST(content AS BLOB), 1, ?), created_at,
		(SELECT COUNT(*) FROM messages WHERE session_id = ? AND committed = 1)
		FROM messages WHERE session_id = ? AND committed = 1 ORDER BY seq DESC LIMIT ?`, byteLimit+1, sessionID, sessionID, messageLimit)
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
		if len(content) > remaining {
			break
		}
		remaining -= len(content)
		message.Content = string(content)
		history = append(history, message)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	slices.Reverse(history)
	return history, available - len(history), nil
}
