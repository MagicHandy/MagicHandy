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
	Snapshot      *MessageSnapshot
	Limit         int
}

// MessageSnapshot is a stateless continuation, not an access credential. Its
// pruning marker detects removal even when the removed row predates the head.
type MessageSnapshot struct {
	Revision       int64 `json:"revision"`
	PrunedRevision int64 `json:"pruned_revision"`
	FirstSeq       int64 `json:"first_seq"`
}

// MessagePage describes one short database snapshot. NextRevision covers only
// delivered changes. Snapshot, when present, must accompany the next request so
// a partial reset below a deletion marker does not restart the same first page.
type MessagePage struct {
	SessionID      string           `json:"session_id"`
	Messages       []LogMessage     `json:"messages"`
	LatestSeq      int64            `json:"latest_seq"`
	FirstSeq       int64            `json:"first_seq"`
	Revision       int64            `json:"revision"`
	NextRevision   int64            `json:"next_revision"`
	Snapshot       *MessageSnapshot `json:"snapshot,omitempty"`
	HasMore        bool             `json:"has_more"`
	Reset          bool             `json:"reset"`
	HistoryGap     bool             `json:"history_gap"`
	HistoryLimit   int              `json:"history_limit"`
	Cursor         int64            `json:"cursor"`
	CursorRevision int64            `json:"cursor_revision"`
}

// ReadMessagePageContext bounds materialized rows and encoded message bytes
// without taking the application's write gate. Bounds, rows and cursor share a
// read transaction, which ends before any HTTP response write.
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
	after, head := messagePageRange(&page, request, resetRevision, prunedRevision)
	limit := request.Limit
	if limit <= 0 || limit > MessageLogCap {
		limit = MessageLogCap
	}
	// Select bounded byte prefixes in SQLite. Scanning a full long row and then
	// slicing it in Go would still allocate the unbounded source first.
	query := messagePageSequenceQuery
	if request.AfterRevision != nil {
		query = messagePageRevisionQuery
	}
	rows, err := tx.QueryContext(ctx, query, request.SessionID, after, head, limit+1)
	if err != nil {
		return page, fmt.Errorf("read chat snapshot messages: %w", err)
	}
	page.Messages, page.HasMore, err = scanBoundedMessagePage(rows, limit)
	closeErr := rows.Close()
	if err != nil {
		return page, err
	}
	if closeErr != nil {
		return page, closeErr
	}
	page.NextRevision = head
	if page.HasMore && request.AfterRevision != nil {
		page.NextRevision = page.Messages[len(page.Messages)-1].Revision
	} else {
		// Once the anchored window is delivered, newer commits are recovered as
		// ordinary deltas. Never acknowledge the newer informational head here.
		page.Snapshot = nil
		page.HasMore = page.HasMore || head < page.Revision
	}
	if err := tx.Commit(); err != nil {
		return page, fmt.Errorf("finish chat snapshot: %w", err)
	}
	return page, nil
}

func messagePageRange(page *MessagePage, request MessagePageRequest, resetRevision, prunedRevision int64) (after, head int64) {
	after, head = max(0, request.AfterSequence), page.Revision
	if request.AfterRevision == nil {
		return
	}
	after = max(0, *request.AfterRevision)
	page.HistoryGap = after > 0 && after < prunedRevision
	if snapshot := request.Snapshot; snapshot != nil && snapshot.Revision > 0 && snapshot.Revision <= head &&
		after > 0 && after < snapshot.Revision && resetRevision <= snapshot.Revision && snapshot.PrunedRevision == prunedRevision && snapshot.FirstSeq == page.FirstSeq {
		page.Snapshot = &MessageSnapshot{Revision: snapshot.Revision, PrunedRevision: prunedRevision, FirstSeq: page.FirstSeq}
		page.HistoryGap = false // Older pruning was already accounted for at reset.
		head = snapshot.Revision
		return
	}
	page.Reset = request.Snapshot != nil || after == 0 || after > head || after < resetRevision || page.HistoryGap
	if page.Reset {
		after = 0
		page.Snapshot = &MessageSnapshot{Revision: head, PrunedRevision: prunedRevision, FirstSeq: page.FirstSeq}
	}
	return
}
