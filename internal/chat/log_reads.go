package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ActiveSessionIDContext is the request-cancellable form of ActiveSessionID.
func (l *MessageLog) ActiveSessionIDContext(ctx context.Context) (string, error) {
	var id string
	err := l.db.SQL().QueryRowContext(ctx, `
		SELECT active_session_id FROM chat_workspace WHERE id = 'current'
	`).Scan(&id)
	if err == sql.ErrNoRows {
		return "", ErrChatSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read active chat session: %w", err)
	}
	return id, nil
}

// SessionsContext is the request-cancellable form of Sessions.
func (l *MessageLog) SessionsContext(ctx context.Context) ([]Session, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `
		SELECT s.id, s.title, s.saved, s.persona_id, s.id = w.active_session_id,
			COUNT(m.seq), COALESCE(MAX(m.seq), 0), s.created_at, s.updated_at
		FROM chat_sessions s
		CROSS JOIN chat_workspace w
		LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE w.id = 'current'
		GROUP BY s.id, s.title, s.saved, s.persona_id, w.active_session_id, s.created_at, s.updated_at
		ORDER BY s.created_at ASC, s.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.Title, &session.Saved, &session.PersonaID, &session.Active,
			&session.MessageCount, &session.LatestSeq, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// SessionContext is the request-cancellable form of Session.
func (l *MessageLog) SessionContext(ctx context.Context, id string) (Session, error) {
	var session Session
	err := l.db.SQL().QueryRowContext(ctx, `
		SELECT s.id, s.title, s.saved, s.persona_id, s.id = w.active_session_id,
			COUNT(m.seq), COALESCE(MAX(m.seq), 0), s.created_at, s.updated_at
		FROM chat_sessions s
		CROSS JOIN chat_workspace w
		LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE s.id = ? AND w.id = 'current'
		GROUP BY s.id, s.title, s.saved, s.persona_id, w.active_session_id, s.created_at, s.updated_at
	`, id).Scan(&session.ID, &session.Title, &session.Saved, &session.PersonaID, &session.Active,
		&session.MessageCount, &session.LatestSeq, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return Session{}, ErrChatSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("read chat session: %w", err)
	}
	return session, nil
}

// AfterSessionContext is the request-cancellable form of AfterSession.
func (l *MessageLog) AfterSessionContext(ctx context.Context, sessionID string, after int64, limit int) ([]LogMessage, error) {
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	rows, err := l.db.SQL().QueryContext(ctx, `
		SELECT seq, role, content, client_id, diagnostics_json, created_at
		FROM messages
		WHERE session_id = ? AND committed = 1 AND seq > ?
		ORDER BY seq ASC
		LIMIT ?
	`, sessionID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanLogMessages(rows)
}

// RecentSessionContext is the request-cancellable form of RecentSession.
func (l *MessageLog) RecentSessionContext(ctx context.Context, sessionID string, limit int) ([]LogMessage, error) {
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	rows, err := l.db.SQL().QueryContext(ctx, `
		SELECT seq, role, content, client_id, diagnostics_json, created_at
		FROM (
			SELECT seq, role, content, client_id, diagnostics_json, created_at
			FROM messages WHERE session_id = ? AND committed = 1 ORDER BY seq DESC LIMIT ?
		) AS recent
		ORDER BY seq ASC
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("read recent chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanLogMessages(rows)
}

// ReadPromptContext is the request-cancellable form of PromptContext.
func (l *MessageLog) ReadPromptContext(ctx context.Context, sessionID string) (SessionPromptContext, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `
		SELECT content, diagnostics_json
		FROM messages
		WHERE session_id = ? AND role = ? AND committed = 1
		ORDER BY seq DESC
		LIMIT ?
	`, sessionID, MessageRoleAssistant, MessageLogCap)
	if err != nil {
		return SessionPromptContext{}, fmt.Errorf("read chat prompt context: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var contextSnapshot SessionPromptContext
	var fallbackMood Mood
	for rows.Next() {
		var content, diagnosticsJSON string
		if err := rows.Scan(&content, &diagnosticsJSON); err != nil {
			return SessionPromptContext{}, fmt.Errorf("scan chat prompt context: %w", err)
		}
		if len(contextSnapshot.RecentAssistantReplies) < maxRecentAssistantReplies {
			if line := boundedPromptData(content, maxRecentAssistantRunes); line != "" {
				contextSnapshot.RecentAssistantReplies = append(contextSnapshot.RecentAssistantReplies, line)
			}
		}
		if contextSnapshot.CurrentMood == "" && diagnosticsJSON != "" && diagnosticsJSON != "{}" {
			var diagnostics MessageDiagnostics
			if err := json.Unmarshal([]byte(diagnosticsJSON), &diagnostics); err != nil {
				return SessionPromptContext{}, fmt.Errorf("decode chat prompt diagnostics: %w", err)
			}
			if mood, ok := validMood(diagnostics.Mood); ok {
				if fallbackMood == "" {
					fallbackMood = mood
				}
				if diagnostics.MoodChanged {
					contextSnapshot.CurrentMood = mood
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return SessionPromptContext{}, fmt.Errorf("read chat prompt context: %w", err)
	}
	if contextSnapshot.CurrentMood == "" {
		contextSnapshot.CurrentMood = fallbackMood
	}
	for left, right := 0, len(contextSnapshot.RecentAssistantReplies)-1; left < right; left, right = left+1, right-1 {
		contextSnapshot.RecentAssistantReplies[left], contextSnapshot.RecentAssistantReplies[right] = contextSnapshot.RecentAssistantReplies[right], contextSnapshot.RecentAssistantReplies[left]
	}
	return contextSnapshot, nil
}

// LatestSeqSessionContext is the request-cancellable form of LatestSeqSession.
func (l *MessageLog) LatestSeqSessionContext(ctx context.Context, sessionID string) (int64, error) {
	var seq sql.NullInt64
	err := l.db.SQL().QueryRowContext(ctx, `
		SELECT MAX(seq) FROM messages WHERE session_id = ? AND committed = 1
	`, sessionID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("read chat session head: %w", err)
	}
	return seq.Int64, nil
}

// CursorSessionContext is the request-cancellable form of CursorSession.
func (l *MessageLog) CursorSessionContext(ctx context.Context, clientID, sessionID string) (int64, error) {
	if clientID == "" {
		return 0, nil
	}
	var seq int64
	err := l.db.SQL().QueryRowContext(ctx, `
		SELECT last_seq FROM chat_session_cursors WHERE client_id = ? AND session_id = ?
	`, clientID, sessionID).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read chat cursor: %w", err)
	}
	return seq, nil
}
