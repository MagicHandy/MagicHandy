package chat

import "context"

// DisplayMessage contains only user-visible conversation text, never prompts,
// diagnostics, speech IDs or browser attribution.
type DisplayMessage struct {
	Seq       int64  `json:"seq"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// RecentDisplayMessages reads a bounded tail of committed messages. Both row
// count and content are bounded in SQLite before Go allocates the result.
func (l *MessageLog) RecentDisplayMessages(ctx context.Context, sessionID string) ([]DisplayMessage, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT seq, role, substr(content,1,2048), length(content)>2048 FROM
		(SELECT seq, role, content FROM messages WHERE session_id = ? AND committed = 1 ORDER BY seq DESC LIMIT 40)
		ORDER BY seq`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]DisplayMessage, 0)
	for rows.Next() {
		var item DisplayMessage
		if err := rows.Scan(&item.Seq, &item.Role, &item.Content, &item.Truncated); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
