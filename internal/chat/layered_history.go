package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// recentUserRequestLimit bounds how many of the latest human lines planning
// turns receive separately from the conversation. Autonomous check-ins can
// crowd a long history window; these lines stay visible regardless.
const recentUserRequestLimit = 8

// RecentUserRequest is one recent human line and when it was committed.
type RecentUserRequest struct {
	Text string
	At   time.Time
}

// RecentUserRequests keeps bounded human intent available independently of
// automated assistant check-ins. The session's normal message retention applies.
func (l *MessageLog) RecentUserRequests(sessionID string) ([]string, error) {
	timeline, err := l.RecentUserRequestTimelineContext(context.Background(), sessionID)
	texts, _ := UserRequestTimeline(timeline, time.Now())
	return texts, err
}

// RecentUserRequestTimelineContext reads bounded human intent, with when each
// line was said, within the caller's lifetime.
func (l *MessageLog) RecentUserRequestTimelineContext(ctx context.Context, sessionID string) ([]RecentUserRequest, error) {
	// Blank rows are skipped below, so read a little past the limit.
	rows, err := l.db.SQL().QueryContext(ctx, `SELECT content, created_at FROM messages WHERE session_id = ? AND role = ? AND committed = 1 ORDER BY seq DESC LIMIT ?`, sessionID, MessageRoleUser, 2*recentUserRequestLimit)
	if err != nil {
		return nil, fmt.Errorf("read user requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var lines []RecentUserRequest
	for rows.Next() {
		var content, created string
		if err := rows.Scan(&content, &created); err != nil {
			return nil, err
		}
		// An unreadable timestamp leaves the line's age unknown, not wrong.
		at, _ := time.Parse(time.RFC3339Nano, created)
		lines = append(lines, RecentUserRequest{Text: content, At: at})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Reverse(lines)
	return SelectRecentUserRequestTimeline(lines), nil
}

// SelectRecentUserRequests returns the latest non-blank human lines, oldest
// first. Every kind of message counts: whether a line is a motion direction, a
// standing wish or small talk is for the model to judge in context, not for a
// word list to decide before it sees them.
func SelectRecentUserRequests(lines []string) []string {
	timeline := make([]RecentUserRequest, len(lines))
	for i, line := range lines {
		timeline[i].Text = line
	}
	texts, _ := UserRequestTimeline(SelectRecentUserRequestTimeline(timeline), time.Time{})
	return texts
}

// SelectRecentUserRequestTimeline is SelectRecentUserRequests for lines that
// carry when they were said.
func SelectRecentUserRequestTimeline(lines []RecentUserRequest) []RecentUserRequest {
	selected := make([]RecentUserRequest, 0, recentUserRequestLimit)
	for i := len(lines) - 1; i >= 0 && len(selected) < recentUserRequestLimit; i-- {
		if text := strings.TrimSpace(lines[i].Text); text != "" {
			selected = append(selected, RecentUserRequest{Text: boundedPromptData(text, 2000), At: lines[i].At})
		}
	}
	slices.Reverse(selected)
	return selected
}

// UserRequestTimeline splits recent lines into the texts and ages a motion
// context carries. Ages are omitted unless every line has a time, so a model
// is never shown a guessed age.
func UserRequestTimeline(lines []RecentUserRequest, now time.Time) ([]string, []int) {
	texts := make([]string, len(lines))
	ages := make([]int, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
		if line.At.IsZero() || now.IsZero() {
			ages = nil
		} else if ages != nil {
			ages[i] = max(0, int(now.Sub(line.At).Seconds()))
		}
	}
	return texts, ages
}
