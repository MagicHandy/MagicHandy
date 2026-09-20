package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// Cursor retention bounds bookkeeping independently from the message window.
// Eviction loses only a read marker; it cannot delete messages or grant access.
const (
	ChatCursorCap       = appstore.ChatCursorLimit
	chatCursorRetention = appstore.ChatCursorRetention
)

// ReadCursor keeps display position distinct from committed recovery order.
type ReadCursor struct {
	Sequence int64
	Revision int64
}

type chatCursorReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readChatCursor(ctx context.Context, reader chatCursorReader, clientID, sessionID string) (ReadCursor, error) {
	var cursor ReadCursor
	if clientID == "" {
		return cursor, nil
	}
	err := reader.QueryRowContext(ctx, `SELECT last_seq, last_revision FROM chat_session_cursors
		WHERE client_id = ? AND session_id = ? AND updated_at >= ?`, clientID, sessionID,
		time.Now().Add(-chatCursorRetention).UTC().Format(time.RFC3339Nano)).Scan(&cursor.Sequence, &cursor.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return cursor, nil
	}
	if err != nil {
		return cursor, fmt.Errorf("read chat cursor: %w", err)
	}
	return cursor, nil
}

// AdvanceCursorSessionContext is the cancelable legacy sequence-only update.
func (l *MessageLog) AdvanceCursorSessionContext(ctx context.Context, clientID, sessionID string, seq int64) (int64, error) {
	cursor, err := l.AdvanceReadCursorContext(ctx, clientID, sessionID, seq, nil)
	return cursor.Sequence, err
}

// AdvanceReadCursorContext cancels while waiting for the writer and clamps both
// read markers to current committed bounds. Passive retries of an unchanged
// marker avoid rewriting it or renewing its retention timestamp.
func (l *MessageLog) AdvanceReadCursorContext(ctx context.Context, clientID, sessionID string, seq int64, revision *int64) (ReadCursor, error) {
	var result ReadCursor
	if clientID == "" || len(clientID) > 256 {
		return result, errors.New("a bounded client id is required to advance a chat cursor")
	}
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var latest, head int64
		err := tx.QueryRowContext(ctx, `SELECT s.revision, COALESCE(MAX(m.seq), 0)
			FROM chat_sessions s LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
			WHERE s.id = ? GROUP BY s.id, s.revision`, sessionID).Scan(&head, &latest)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatSessionNotFound
		}
		if err != nil {
			return err
		}
		previous, err := readChatCursor(ctx, tx, clientID, sessionID)
		if err != nil {
			return err
		}
		result.Sequence = min(latest, max(previous.Sequence, max(0, seq)))
		result.Revision = min(head, previous.Revision)
		if revision != nil {
			result.Revision = min(head, max(result.Revision, max(0, *revision)))
		}
		if result == previous {
			return nil
		}
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_session_cursors WHERE updated_at < ?`,
			now.Add(-chatCursorRetention).Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chat_session_cursors(client_id, session_id, last_seq, last_revision, updated_at)
			VALUES(?, ?, ?, ?, ?) ON CONFLICT(client_id, session_id) DO UPDATE SET
			last_seq = excluded.last_seq, last_revision = excluded.last_revision, updated_at = excluded.updated_at`,
			clientID, sessionID, result.Sequence, result.Revision, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM chat_session_cursors WHERE (client_id, session_id) IN (
			SELECT client_id, session_id FROM chat_session_cursors ORDER BY updated_at DESC, client_id DESC, session_id DESC
			LIMIT -1 OFFSET ?)`, ChatCursorCap)
		return err
	})
	if err != nil {
		return ReadCursor{}, fmt.Errorf("advance chat cursor: %w", err)
	}
	return result, nil
}
