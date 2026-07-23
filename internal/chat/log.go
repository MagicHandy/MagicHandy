package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// MessageLogCap bounds each chat session independently.
const MessageLogCap = 200

// Message roles accepted by the visible chat log.
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	defaultSessionTitle  = "New chat"
)

// Session lifecycle errors returned by MessageLog mutations.
var (
	ErrChatSessionNotFound    = errors.New("chat session not found")
	ErrActiveSessionDelete    = errors.New("the active chat session cannot be deleted")
	ErrUnsavedSessionConflict = errors.New("the active unsaved chat must be saved or discarded first")
)

// MessageDiagnostics is the non-secret provenance retained with one visible
// assistant reply. Prompts, memories, request bodies, and credentials never
// enter this payload.
type MessageDiagnostics struct {
	Source           string `json:"source,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptSet        string `json:"prompt_set,omitempty"`
	RequestMillis    int64  `json:"request_ms,omitempty"`
	Repaired         bool   `json:"repaired,omitempty"`
	SemanticFallback bool   `json:"semantic_fallback,omitempty"`
	InitialMalformed bool   `json:"initial_malformed,omitempty"`
	MotionAction     string `json:"motion_action,omitempty"`
	Mood             Mood   `json:"mood,omitempty"`
	MoodChanged      bool   `json:"mood_changed,omitempty"`
}

// SessionPromptContext is the backend-owned per-session continuity snapshot
// supplied to one model turn.
type SessionPromptContext struct {
	CurrentMood            Mood
	RecentAssistantReplies []string
}

// LogMessage is one visible row in a chat session.
type LogMessage struct {
	Seq             int64               `json:"seq"`
	Role            string              `json:"role"`
	Content         string              `json:"content"`
	ClientID        string              `json:"client_id,omitempty"`
	CreatedAt       string              `json:"created_at"`
	Diagnostics     *MessageDiagnostics `json:"diagnostics,omitempty"`
	SpeechRequestID string              `json:"speech_request_id,omitempty"`
}

// Session is one retained or process-local conversation tab. Exactly one row
// is active through the chat_workspace singleton.
type Session struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Saved        bool   `json:"saved"`
	Active       bool   `json:"active"`
	MessageCount int    `json:"message_count"`
	LatestSeq    int64  `json:"latest_seq"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// MessageLog owns chat sessions, their bounded messages, and per-client,
// per-session cursors in the process datastore.
type MessageLog struct {
	db     *dbstore.DB
	ownsDB bool
}

// OpenMessageLog opens a chat log that owns its datastore connection.
func OpenMessageLog(dataDir string) (*MessageLog, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open chat message log: %w", err)
	}
	log := &MessageLog{db: database, ownsDB: true}
	if err := log.discardPending(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return log, nil
}

// OpenMessageLogWithDatabase opens a chat log over the process datastore.
func OpenMessageLogWithDatabase(database *dbstore.DB) (*MessageLog, error) {
	if database == nil {
		return nil, errors.New("chat message datastore is required")
	}
	log := &MessageLog{db: database}
	if err := log.discardPending(); err != nil {
		return nil, err
	}
	return log, nil
}

func (l *MessageLog) discardPending() error {
	if err := l.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM messages WHERE committed = 0`)
		return err
	}); err != nil {
		return fmt.Errorf("discard interrupted chat replies: %w", err)
	}
	return nil
}

// Close releases the datastore only when the log opened it.
func (l *MessageLog) Close() error {
	if !l.ownsDB {
		return nil
	}
	return l.db.Close()
}

// ReconcileStartup applies the configured restart policy once when the server
// opens its persistent domains. Saved sessions are never removed.
func (l *MessageLog) ReconcileStartup(startupBehavior string, keepUnsaved bool) (Session, error) {
	ctx := context.Background()
	var activeID string
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var currentID string
		var currentSaved bool
		err := tx.QueryRowContext(ctx, `
			SELECT s.id, s.saved
			FROM chat_workspace w
			JOIN chat_sessions s ON s.id = w.active_session_id
			WHERE w.id = 'current'
		`).Scan(&currentID, &currentSaved)
		if err != nil && err != sql.ErrNoRows {
			return err
		}

		if startupBehavior == "previous" && currentID != "" && (currentSaved || keepUnsaved) {
			activeID = currentID
		}
		if startupBehavior == "previous" && activeID == "" {
			_ = tx.QueryRowContext(ctx, `
				SELECT id FROM chat_sessions WHERE saved = 1
				ORDER BY updated_at DESC, id DESC LIMIT 1
			`).Scan(&activeID)
		}
		if startupBehavior == "new" || activeID == "" {
			var err error
			activeID, err = insertSession(ctx, tx)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_workspace(id, active_session_id, updated_at)
			VALUES('current', ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				active_session_id = excluded.active_session_id,
				updated_at = excluded.updated_at
		`, activeID, nowUTC()); err != nil {
			return err
		}
		// Saved tabs plus one unsaved working tab are the whole retained
		// workspace. This also cleans up drafts created by older builds that
		// allowed an unsaved tab to become inactive.
		_, err = tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE saved = 0 AND id <> ?`, activeID)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("reconcile chat startup: %w", err)
	}
	return l.Session(activeID)
}

// ReconcileShutdown removes an unsaved working conversation during a clean
// shutdown unless the user explicitly chose to retain it. Startup performs
// the same reconciliation because a killed process cannot run this hook.
func (l *MessageLog) ReconcileShutdown(keepUnsaved bool) error {
	if keepUnsaved {
		return nil
	}
	if _, err := l.ReconcileStartup("previous", false); err != nil {
		return fmt.Errorf("reconcile chat shutdown: %w", err)
	}
	return nil
}

// ActiveSessionID returns the singleton workspace's selected session.
func (l *MessageLog) ActiveSessionID() (string, error) {
	var id string
	err := l.db.SQL().QueryRowContext(context.Background(), `
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

// Sessions lists retained tabs in stable creation order.
func (l *MessageLog) Sessions() ([]Session, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT s.id, s.title, s.saved, s.id = w.active_session_id,
			COUNT(m.seq), COALESCE(MAX(m.seq), 0), s.created_at, s.updated_at
		FROM chat_sessions s
		CROSS JOIN chat_workspace w
		LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE w.id = 'current'
		GROUP BY s.id, s.title, s.saved, w.active_session_id, s.created_at, s.updated_at
		ORDER BY s.created_at ASC, s.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.Title, &session.Saved, &session.Active,
			&session.MessageCount, &session.LatestSeq, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// Session returns one retained tab and its current summary.
func (l *MessageLog) Session(id string) (Session, error) {
	var session Session
	err := l.db.SQL().QueryRowContext(context.Background(), `
		SELECT s.id, s.title, s.saved, s.id = w.active_session_id,
			COUNT(m.seq), COALESCE(MAX(m.seq), 0), s.created_at, s.updated_at
		FROM chat_sessions s
		CROSS JOIN chat_workspace w
		LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE s.id = ? AND w.id = 'current'
		GROUP BY s.id, s.title, s.saved, w.active_session_id, s.created_at, s.updated_at
	`, id).Scan(&session.ID, &session.Title, &session.Saved, &session.Active,
		&session.MessageCount, &session.LatestSeq, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return Session{}, ErrChatSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("read chat session: %w", err)
	}
	return session, nil
}

// CreateSession selects a new unsaved conversation. discardCurrentUnsaved is
// used only after explicit UI confirmation; saved sessions are never deleted.
func (l *MessageLog) CreateSession(discardCurrentUnsaved bool) (Session, error) {
	ctx := context.Background()
	var id string
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var currentID string
		var currentSaved bool
		var currentCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT s.id, s.saved, COUNT(m.seq)
			FROM chat_workspace w
			JOIN chat_sessions s ON s.id = w.active_session_id
			LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
			WHERE w.id = 'current'
			GROUP BY s.id, s.saved
		`).Scan(&currentID, &currentSaved, &currentCount); err != nil {
			return err
		}
		if !currentSaved && currentCount > 0 && !discardCurrentUnsaved {
			return ErrUnsavedSessionConflict
		}
		var err error
		id, err = insertSession(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE chat_workspace SET active_session_id = ?, updated_at = ? WHERE id = 'current'
		`, id, nowUTC()); err != nil {
			return err
		}
		if !currentSaved && (discardCurrentUnsaved || currentCount == 0) {
			_, err = tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, currentID)
			return err
		}
		return nil
	})
	if err != nil {
		return Session{}, fmt.Errorf("create chat session: %w", err)
	}
	return l.Session(id)
}

// ActivateSession selects one tab and optionally discards the prior unsaved tab.
func (l *MessageLog) ActivateSession(id string, discardCurrentUnsaved bool) (Session, error) {
	ctx := context.Background()
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var currentID string
		var currentSaved bool
		var currentCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT s.id, s.saved, COUNT(m.seq)
			FROM chat_workspace w
			JOIN chat_sessions s ON s.id = w.active_session_id
			LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
			WHERE w.id = 'current'
			GROUP BY s.id, s.saved
		`).Scan(&currentID, &currentSaved, &currentCount); err != nil {
			return err
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM chat_sessions WHERE id = ?`, id).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return ErrChatSessionNotFound
			}
			return err
		}
		if currentID != id && !currentSaved && currentCount > 0 && !discardCurrentUnsaved {
			return ErrUnsavedSessionConflict
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE chat_workspace SET active_session_id = ?, updated_at = ? WHERE id = 'current'
		`, id, nowUTC()); err != nil {
			return err
		}
		if !currentSaved && currentID != id && (discardCurrentUnsaved || currentCount == 0) {
			_, err := tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, currentID)
			return err
		}
		return nil
	})
	if err != nil {
		return Session{}, fmt.Errorf("activate chat session: %w", err)
	}
	return l.Session(id)
}

// SaveSession marks one tab for retention across process starts.
func (l *MessageLog) SaveSession(id string) (Session, error) {
	ctx := context.Background()
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE chat_sessions SET saved = 1, updated_at = ? WHERE id = ?`, nowUTC(), id)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrChatSessionNotFound
		}
		return nil
	})
	if err != nil {
		return Session{}, fmt.Errorf("save chat session: %w", err)
	}
	return l.Session(id)
}

// DeleteSession removes an inactive retained tab and its dependent rows.
func (l *MessageLog) DeleteSession(id string) error {
	ctx := context.Background()
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var activeID string
		if err := tx.QueryRowContext(ctx, `SELECT active_session_id FROM chat_workspace WHERE id = 'current'`).Scan(&activeID); err != nil {
			return err
		}
		if id == activeID {
			return ErrActiveSessionDelete
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrChatSessionNotFound
		}
		return nil
	}); err != nil {
		return fmt.Errorf("delete chat session: %w", err)
	}
	return nil
}

func insertSession(ctx context.Context, tx *sql.Tx) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	now := nowUTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_sessions(id, title, saved, created_at, updated_at)
		VALUES(?, ?, 0, ?, ?)
	`, id, defaultSessionTitle, now, now)
	return id, err
}

func newSessionID() (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate chat session id: %w", err)
	}
	return "chat-" + hex.EncodeToString(buffer), nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func chatTitle(content string) string {
	value := strings.Join(strings.Fields(content), " ")
	runes := []rune(value)
	if len(runes) > 42 {
		value = string(runes[:39]) + "..."
	}
	if value == "" {
		return defaultSessionTitle
	}
	return value
}

// Append adds to the active session. New code that binds a streaming request
// to a session should use AppendTo so a tab switch cannot redirect its reply.
func (l *MessageLog) Append(role string, content string, clientID string) (int64, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return 0, err
	}
	return l.AppendTo(sessionID, role, content, clientID, nil)
}

// AppendTo adds one visible message to the selected session.
func (l *MessageLog) AppendTo(sessionID, role, content, clientID string, diagnostics *MessageDiagnostics) (int64, error) {
	return l.appendTo(sessionID, role, content, clientID, diagnostics, true)
}

// AppendPendingAssistantTo stages one generated reply. Reads and cap pruning
// ignore it until CommitPending makes the row visible.
func (l *MessageLog) AppendPendingAssistantTo(sessionID, content string, diagnostics *MessageDiagnostics) (int64, error) {
	return l.appendTo(sessionID, MessageRoleAssistant, content, "", diagnostics, false)
}

func (l *MessageLog) appendTo(sessionID, role, content, clientID string, diagnostics *MessageDiagnostics, committed bool) (int64, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, fmt.Errorf("chat log rejects empty %s messages", role)
	}
	if role != MessageRoleUser && role != MessageRoleAssistant {
		return 0, fmt.Errorf("chat log rejects role %q", role)
	}
	diagnosticsJSON := []byte("{}")
	var err error
	if diagnostics != nil {
		diagnosticsJSON, err = json.Marshal(diagnostics)
		if err != nil {
			return 0, fmt.Errorf("encode chat diagnostics: %w", err)
		}
	}

	ctx := context.Background()
	var seq int64
	err = l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM chat_sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return ErrChatSessionNotFound
			}
			return err
		}
		now := nowUTC()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO messages(session_id, role, content, client_id, diagnostics_json, created_at, committed)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`, sessionID, role, content, clientID, string(diagnosticsJSON), now, committed)
		if err != nil {
			return err
		}
		seq, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if !committed {
			return nil
		}
		if role == MessageRoleUser {
			_, err = tx.ExecContext(ctx, `
				UPDATE chat_sessions
				SET title = CASE WHEN title = ? THEN ? ELSE title END, updated_at = ?
				WHERE id = ?
			`, defaultSessionTitle, chatTitle(content), now, sessionID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = ? WHERE id = ?`, now, sessionID)
		}
		if err != nil {
			return err
		}
		return pruneCommittedMessages(ctx, tx, sessionID)
	})
	if err != nil {
		return 0, fmt.Errorf("append chat message: %w", err)
	}
	return seq, nil
}

// CommitPending atomically exposes one staged assistant reply and applies the
// per-session cap. A Stop that wins the caller's commit barrier deletes the
// staged row instead, so a canceled reply cannot evict visible history.
func (l *MessageLog) CommitPending(seq int64) error {
	if seq <= 0 {
		return errors.New("a pending chat sequence is required")
	}
	ctx := context.Background()
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var sessionID string
		if err := tx.QueryRowContext(ctx, `
			SELECT session_id FROM messages WHERE seq = ? AND committed = 0
		`, seq).Scan(&sessionID); err != nil {
			if err == sql.ErrNoRows {
				return errors.New("pending chat reply not found")
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET committed = 1 WHERE seq = ?`, seq); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = ? WHERE id = ?`, nowUTC(), sessionID); err != nil {
			return err
		}
		return pruneCommittedMessages(ctx, tx, sessionID)
	}); err != nil {
		return fmt.Errorf("commit pending chat reply: %w", err)
	}
	return nil
}

func pruneCommittedMessages(ctx context.Context, tx *sql.Tx, sessionID string) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM messages
		WHERE session_id = ? AND committed = 1 AND seq NOT IN (
			SELECT seq FROM messages
			WHERE session_id = ? AND committed = 1
			ORDER BY seq DESC LIMIT ?
		)
	`, sessionID, sessionID, MessageLogCap)
	return err
}

// Delete removes one message by its process-wide sequence number.
func (l *MessageLog) Delete(seq int64) error {
	if seq <= 0 {
		return nil
	}
	ctx := context.Background()
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE seq = ?`, seq)
		return err
	}); err != nil {
		return fmt.Errorf("delete chat message: %w", err)
	}
	return nil
}

// After returns active-session messages after a sequence number.
func (l *MessageLog) After(after int64, limit int) ([]LogMessage, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return nil, err
	}
	return l.AfterSession(sessionID, after, limit)
}

// AfterSession returns selected-session messages after a sequence number.
func (l *MessageLog) AfterSession(sessionID string, after int64, limit int) ([]LogMessage, error) {
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	rows, err := l.db.SQL().QueryContext(context.Background(), `
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

// Recent returns the active session's newest messages in chronological order.
func (l *MessageLog) Recent(limit int) ([]LogMessage, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return nil, err
	}
	return l.RecentSession(sessionID, limit)
}

// RecentSession returns one session's newest messages in chronological order.
func (l *MessageLog) RecentSession(sessionID string, limit int) ([]LogMessage, error) {
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	rows, err := l.db.SQL().QueryContext(context.Background(), `
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

// PromptContext returns the effective mood and latest assistant lines for one
// session. Values are projected and bounded for prompts; stored replies remain
// unchanged.
func (l *MessageLog) PromptContext(sessionID string) (SessionPromptContext, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `
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

func scanLogMessages(rows *sql.Rows) ([]LogMessage, error) {
	var messages []LogMessage
	for rows.Next() {
		var message LogMessage
		var diagnosticsJSON string
		if err := rows.Scan(&message.Seq, &message.Role, &message.Content, &message.ClientID, &diagnosticsJSON, &message.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		if diagnosticsJSON != "" && diagnosticsJSON != "{}" {
			var diagnostics MessageDiagnostics
			if err := json.Unmarshal([]byte(diagnosticsJSON), &diagnostics); err != nil {
				return nil, fmt.Errorf("decode chat message diagnostics: %w", err)
			}
			message.Diagnostics = &diagnostics
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// LatestSeq returns the active session's newest sequence number.
func (l *MessageLog) LatestSeq() (int64, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return 0, err
	}
	return l.LatestSeqSession(sessionID)
}

// LatestSeqSession returns one session's newest sequence number.
func (l *MessageLog) LatestSeqSession(sessionID string) (int64, error) {
	var seq sql.NullInt64
	err := l.db.SQL().QueryRowContext(context.Background(), `
		SELECT MAX(seq) FROM messages WHERE session_id = ? AND committed = 1
	`, sessionID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("read chat session head: %w", err)
	}
	return seq.Int64, nil
}

// Cursor returns a client's read position for the active session.
func (l *MessageLog) Cursor(clientID string) (int64, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return 0, err
	}
	return l.CursorSession(clientID, sessionID)
}

// CursorSession returns a client's read position for one session.
func (l *MessageLog) CursorSession(clientID, sessionID string) (int64, error) {
	if clientID == "" {
		return 0, nil
	}
	var seq int64
	err := l.db.SQL().QueryRowContext(context.Background(), `
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

// AdvanceCursor moves a client's active-session cursor monotonically forward.
func (l *MessageLog) AdvanceCursor(clientID string, seq int64) (int64, error) {
	sessionID, err := l.ActiveSessionID()
	if err != nil {
		return 0, err
	}
	return l.AdvanceCursorSession(clientID, sessionID, seq)
}

// AdvanceCursorSession moves a client's selected-session cursor forward.
func (l *MessageLog) AdvanceCursorSession(clientID, sessionID string, seq int64) (int64, error) {
	if clientID == "" {
		return 0, errors.New("a client id is required to advance a chat cursor")
	}
	if seq < 0 {
		seq = 0
	}
	ctx := context.Background()
	var stored int64
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var latest sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM messages WHERE session_id = ? AND committed = 1`, sessionID).Scan(&latest); err != nil {
			return err
		}
		if seq > latest.Int64 {
			seq = latest.Int64
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO chat_session_cursors(client_id, session_id, last_seq, updated_at)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(client_id, session_id) DO UPDATE SET
				last_seq = MIN(?, MAX(chat_session_cursors.last_seq, excluded.last_seq)),
				updated_at = excluded.updated_at
		`, clientID, sessionID, seq, nowUTC(), latest.Int64)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
			SELECT last_seq FROM chat_session_cursors WHERE client_id = ? AND session_id = ?
		`, clientID, sessionID).Scan(&stored)
	})
	if err != nil {
		return 0, fmt.Errorf("advance chat cursor: %w", err)
	}
	return stored, nil
}

// Clear resets every chat session and cursor. It is test support; settings
// reset intentionally does not erase conversations.
func (l *MessageLog) Clear() error {
	ctx := context.Background()
	id, err := newSessionID()
	if err != nil {
		return err
	}
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{
			`DELETE FROM chat_session_cursors`,
			`DELETE FROM client_cursors`,
			`DELETE FROM messages`,
			`DELETE FROM chat_workspace`,
			`DELETE FROM chat_sessions`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		now := nowUTC()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_sessions(id, title, saved, created_at, updated_at) VALUES(?, ?, 0, ?, ?)
		`, id, defaultSessionTitle, now, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO chat_workspace(id, active_session_id, updated_at) VALUES('current', ?, ?)
		`, id, now)
		return err
	})
}
