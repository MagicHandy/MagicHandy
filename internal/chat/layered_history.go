package chat

import (
	"context"
	"fmt"
)

// RecentUserRequests keeps bounded human intent available independently of
// automated assistant check-ins. The session's normal message retention applies.
func (l *MessageLog) RecentUserRequests(sessionID string) ([]string, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `SELECT content FROM (SELECT seq, content FROM messages WHERE session_id = ? AND role = ? AND committed = 1 ORDER BY seq DESC LIMIT 4) ORDER BY seq ASC`, sessionID, MessageRoleUser)
	if err != nil {
		return nil, fmt.Errorf("read user requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	requests := []string{}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		requests = append(requests, boundedPromptData(content, 2000))
	}
	return requests, rows.Err()
}
