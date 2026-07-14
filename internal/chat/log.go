package chat

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// MessageLogCap bounds the shared chat log. Appends prune the oldest rows
// past this cap so the log cannot grow without bound.
const MessageLogCap = 200

// Message roles in the shared log.
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
)

// LogMessage is one row of the shared chat message log — the single
// canonical history every client reads via its own cursor (ADR 0003).
// Model/transport errors are never appended: only text a user typed or a
// reply that was actually displayed.
type LogMessage struct {
	Seq       int64  `json:"seq"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ClientID  string `json:"client_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

// MessageLog is the DB-backed shared chat history with per-client cursors.
// Reads are non-destructive: one client reading never consumes another
// client's view of the log.
type MessageLog struct {
	db *dbstore.DB
}

// OpenMessageLog opens the shared chat log in the app datastore.
func OpenMessageLog(dataDir string) (*MessageLog, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open chat message log: %w", err)
	}
	return &MessageLog{db: database}, nil
}

// Close releases the log's database handle.
func (l *MessageLog) Close() error {
	return l.db.Close()
}

// Append adds one message and prunes the log to MessageLogCap. It returns
// the assigned sequence number.
func (l *MessageLog) Append(role string, content string, clientID string) (int64, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, fmt.Errorf("chat log rejects empty %s messages", role)
	}
	if role != MessageRoleUser && role != MessageRoleAssistant {
		return 0, fmt.Errorf("chat log rejects role %q", role)
	}

	ctx := context.Background()
	var seq int64
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO messages(role, content, client_id, created_at)
			VALUES(?, ?, ?, ?)
		`, role, content, clientID, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		seq, err = result.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			DELETE FROM messages
			WHERE seq <= (SELECT MAX(seq) FROM messages) - ?
		`, MessageLogCap)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("append chat message: %w", err)
	}
	return seq, nil
}

// Delete removes one message that was invalidated before it became visible.
// Sequence numbers are intentionally not reused.
func (l *MessageLog) Delete(seq int64) error {
	if seq <= 0 {
		return nil
	}
	if _, err := l.db.SQL().ExecContext(context.Background(), `DELETE FROM messages WHERE seq = ?`, seq); err != nil {
		return fmt.Errorf("delete chat message: %w", err)
	}
	return nil
}

// After returns messages with seq greater than after, oldest first, capped
// at limit (or the full bounded log when limit <= 0).
func (l *MessageLog) After(after int64, limit int) ([]LogMessage, error) {
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT seq, role, content, client_id, created_at
		FROM messages
		WHERE seq > ?
		ORDER BY seq ASC
		LIMIT ?
	`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []LogMessage
	for rows.Next() {
		var message LogMessage
		if err := rows.Scan(&message.Seq, &message.Role, &message.Content, &message.ClientID, &message.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// LatestSeq returns the newest sequence number (0 for an empty log).
func (l *MessageLog) LatestSeq() (int64, error) {
	var seq sql.NullInt64
	err := l.db.SQL().QueryRowContext(context.Background(),
		`SELECT MAX(seq) FROM messages`).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("read chat log head: %w", err)
	}
	return seq.Int64, nil
}

// Cursor returns the stored read position for one client (0 if none).
func (l *MessageLog) Cursor(clientID string) (int64, error) {
	if clientID == "" {
		return 0, nil
	}
	var seq int64
	err := l.db.SQL().QueryRowContext(context.Background(), `
		SELECT last_seq FROM client_cursors WHERE client_id = ?
	`, clientID).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read chat cursor: %w", err)
	}
	return seq, nil
}

// AdvanceCursor moves a client's cursor forward, never backward, so one
// misbehaving request cannot make a client re-consume history it acked.
func (l *MessageLog) AdvanceCursor(clientID string, seq int64) (int64, error) {
	if clientID == "" {
		return 0, fmt.Errorf("a client id is required to advance a chat cursor")
	}
	if seq < 0 {
		seq = 0
	}
	ctx := context.Background()
	var stored int64
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO client_cursors(client_id, last_seq, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(client_id) DO UPDATE SET
				last_seq = MAX(client_cursors.last_seq, excluded.last_seq),
				updated_at = excluded.updated_at
		`, clientID, seq, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
			SELECT last_seq FROM client_cursors WHERE client_id = ?
		`, clientID).Scan(&stored)
	})
	if err != nil {
		return 0, fmt.Errorf("advance chat cursor: %w", err)
	}
	return stored, nil
}

// Clear removes all messages and cursors (used by tests and future reset
// affordances; settings reset does not touch chat history).
func (l *MessageLog) Clear() error {
	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM client_cursors`)
		return err
	})
}
