package chat

import (
	"context"
	"database/sql"
)

// Display sequences are allocated before a pending reply becomes visible.
// Recovery revisions advance only when a committed change becomes durable.
func publishMessageRevision(ctx context.Context, tx *sql.Tx, sessionID string, seq int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE chat_sessions SET revision = revision + 1 WHERE id = ?`, sessionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE messages SET revision = (SELECT revision FROM chat_sessions WHERE id = ?) WHERE seq = ?`, sessionID, seq)
	return err
}

// ConversationHeadContext reads display and recovery heads in one snapshot.
func (l *MessageLog) ConversationHeadContext(ctx context.Context, sessionID string) (seq, revision int64, err error) {
	err = l.db.SQL().QueryRowContext(ctx, `SELECT s.revision, COALESCE(MAX(m.seq), 0)
		FROM chat_sessions s LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE s.id = ? GROUP BY s.id, s.revision`, sessionID).Scan(&revision, &seq)
	if err == sql.ErrNoRows {
		err = ErrChatSessionNotFound
	}
	return
}
