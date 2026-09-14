package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ChatCursorLimit caps stored read markers. ChatCursorRetention governs normal
// reads and advancement; migration preserves legacy markers within the cap.
const (
	ChatCursorLimit     = 4096
	ChatCursorRetention = 24 * time.Hour
)

// Like the earlier chat migrations, tolerate a rewound schema version without
// resetting revisions already written by a newer binary.
func migrateChatRecovery(ctx context.Context, tx *sql.Tx) error {
	for _, column := range []struct{ table, name string }{
		{"chat_sessions", "revision"}, {"chat_sessions", "reset_revision"},
		{"chat_sessions", "pruned_revision"}, {"messages", "revision"},
		{"chat_session_cursors", "last_revision"},
	} {
		exists, err := columnExists(ctx, tx, column.table, column.name)
		if err != nil {
			return err
		}
		if !exists {
			// Identifiers come exclusively from the fixed table above.
			statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s INTEGER NOT NULL DEFAULT 0", column.table, column.name)
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
			if column.table == "messages" {
				if _, err := tx.ExecContext(ctx, `UPDATE messages SET revision = seq WHERE committed = 1`); err != nil {
					return err
				}
			}
		}
	}
	for _, statement := range []string{
		`UPDATE chat_sessions SET revision = MAX(revision, COALESCE(
			(SELECT MAX(revision) FROM messages WHERE session_id = chat_sessions.id AND committed = 1), 0))`,
		`CREATE INDEX IF NOT EXISTS messages_session_revision ON messages(session_id, committed, revision)`,
		`CREATE INDEX IF NOT EXISTS chat_cursors_updated ON chat_session_cursors(updated_at, client_id, session_id)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM chat_session_cursors WHERE (client_id, session_id) IN (
		SELECT client_id, session_id FROM chat_session_cursors ORDER BY updated_at DESC, client_id DESC, session_id DESC
		LIMIT -1 OFFSET ?)`, ChatCursorLimit)
	return err
}
