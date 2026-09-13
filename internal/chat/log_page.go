package chat

import (
	"context"
	"database/sql"
	"fmt"
)

// MessagePageRequest chooses legacy display order or committed recovery order.
type MessagePageRequest struct {
	SessionID     string
	ClientID      string
	AfterSequence int64
	AfterRevision *int64
	Limit         int
}

// MessagePage describes one committed database snapshot. NextRevision covers
// only delivered changes; Revision is the informational conversation head.
type MessagePage struct {
	SessionID      string       `json:"session_id"`
	Messages       []LogMessage `json:"messages"`
	LatestSeq      int64        `json:"latest_seq"`
	FirstSeq       int64        `json:"first_seq"`
	Revision       int64        `json:"revision"`
	NextRevision   int64        `json:"next_revision"`
	HasMore        bool         `json:"has_more"`
	Reset          bool         `json:"reset"`
	HistoryGap     bool         `json:"history_gap"`
	HistoryLimit   int          `json:"history_limit"`
	Cursor         int64        `json:"cursor"`
	CursorRevision int64        `json:"cursor_revision"`
}

// ReadMessagePageContext uses a short read transaction, without taking the
// application's write gate. Rows, retained bounds, head and cursor cannot come
// from different commits. No transaction survives an HTTP response write.
func (l *MessageLog) ReadMessagePageContext(ctx context.Context, request MessagePageRequest) (MessagePage, error) {
	page := MessagePage{SessionID: request.SessionID, Messages: []LogMessage{}, HistoryLimit: MessageLogCap}
	tx, err := l.db.SQL().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, fmt.Errorf("open chat snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var resetRevision, prunedRevision int64
	err = tx.QueryRowContext(ctx, `SELECT s.revision, s.reset_revision, s.pruned_revision,
		COALESCE(MIN(m.seq), 0), COALESCE(MAX(m.seq), 0)
		FROM chat_sessions s LEFT JOIN messages m ON m.session_id = s.id AND m.committed = 1
		WHERE s.id = ? GROUP BY s.id, s.revision, s.reset_revision, s.pruned_revision`, request.SessionID).
		Scan(&page.Revision, &resetRevision, &prunedRevision, &page.FirstSeq, &page.LatestSeq)
	if err == sql.ErrNoRows {
		return page, ErrChatSessionNotFound
	}
	if err != nil {
		return page, fmt.Errorf("read chat snapshot bounds: %w", err)
	}
	cursor, err := readChatCursor(ctx, tx, request.ClientID, request.SessionID)
	if err != nil {
		return page, err
	}
	page.Cursor, page.CursorRevision = min(cursor.Sequence, page.LatestSeq), min(cursor.Revision, page.Revision)
	query, after, limit := messagePageQuery(&page, request, resetRevision, prunedRevision)
	rows, err := tx.QueryContext(ctx, query, request.SessionID, after, limit+1)
	if err != nil {
		return page, fmt.Errorf("read chat snapshot messages: %w", err)
	}
	messages, readErr := scanLogMessageRows(rows, true)
	closeErr := rows.Close()
	if readErr != nil {
		return page, readErr
	}
	if closeErr != nil {
		return page, closeErr
	}
	page.HasMore = len(messages) > limit
	if page.HasMore {
		messages = messages[:limit]
	}
	if messages != nil {
		page.Messages = messages
	}
	page.NextRevision = page.Revision
	if page.HasMore && request.AfterRevision != nil {
		page.NextRevision = messages[len(messages)-1].Revision
	}
	if err := tx.Commit(); err != nil {
		return page, fmt.Errorf("finish chat snapshot: %w", err)
	}
	return page, nil
}

func messagePageQuery(page *MessagePage, request MessagePageRequest, resetRevision, prunedRevision int64) (query string, after int64, limit int) {
	limit = request.Limit
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	after = max(0, request.AfterSequence)
	query = `SELECT seq, role, content, client_id, diagnostics_json, created_at, revision
		FROM messages WHERE session_id = ? AND committed = 1 AND seq > ? ORDER BY seq ASC LIMIT ?`
	if request.AfterRevision == nil {
		return
	}
	after = max(0, *request.AfterRevision)
	page.HistoryGap = after > 0 && after < prunedRevision
	page.Reset = after == 0 || after > page.Revision || after < resetRevision || page.HistoryGap
	if page.Reset {
		after = 0
		// Resets deliver the complete retained window. Splitting a reset below
		// reset_revision would repeatedly restart pagination.
		limit = MessageLogCap
	}
	query = `SELECT seq, role, content, client_id, diagnostics_json, created_at, revision
		FROM messages WHERE session_id = ? AND committed = 1 AND revision > ? ORDER BY revision ASC, seq ASC LIMIT ?`
	return
}
