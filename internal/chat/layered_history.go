package chat

import (
	"context"
	"fmt"
	"strings"
)

// recentUserRequestLimit bounds how many of the latest human lines planning
// turns receive separately from the conversation. Autonomous check-ins can
// crowd a long history window; these lines stay visible regardless.
const recentUserRequestLimit = 8

// RecentUserRequests keeps bounded human intent available independently of
// automated assistant check-ins. The session's normal message retention applies.
func (l *MessageLog) RecentUserRequests(sessionID string) ([]string, error) {
	return l.RecentUserRequestsContext(context.Background(), sessionID)
}

// RecentUserRequestsContext reads bounded human intent with the caller's lifetime.
func (l *MessageLog) RecentUserRequestsContext(ctx context.Context, sessionID string) ([]string, error) {
	// Blank rows are skipped below, so read a little past the limit.
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT content FROM messages WHERE session_id = ? AND role = ? AND committed = 1 ORDER BY seq DESC LIMIT ?`, sessionID, MessageRoleUser, 2*recentUserRequestLimit)
	if err != nil {
		return nil, fmt.Errorf("read user requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var newestFirst []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		newestFirst = append(newestFirst, content)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(newestFirst))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		lines = append(lines, newestFirst[i])
	}
	return SelectRecentUserRequests(lines), nil
}

// SelectRecentUserRequests returns the latest non-blank human lines, oldest
// first. Every kind of message counts: whether a line is a motion direction, a
// standing wish or small talk is for the model to judge in context, not for a
// word list to decide before it sees them.
func SelectRecentUserRequests(lines []string) []string {
	selected := make([]string, 0, recentUserRequestLimit)
	for i := len(lines) - 1; i >= 0 && len(selected) < recentUserRequestLimit; i-- {
		if text := strings.TrimSpace(lines[i]); text != "" {
			selected = append(selected, boundedPromptData(text, 2000))
		}
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected
}
