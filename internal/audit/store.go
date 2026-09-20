package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	appstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// Retention and response limits bound the durable history and each read.
const (
	RetentionDays = 30
	PageSize      = 100
	PageMaxBytes  = 256 << 10
)

// AppendTx makes an access mutation and its audit record one durable outcome.
// Callers must propagate failure so the transaction cannot silently omit history.
func AppendTx(ctx context.Context, tx *sql.Tx, event Event) error {
	if event.Actor.Type == "" {
		event.Actor = ContextActor(ctx)
	}
	event, data, err := prepare(event)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO access_audit(event_id, occurred_at, document) VALUES(?, ?, ?) ON CONFLICT(event_id) DO NOTHING`, event.ID, event.OccurredAt, string(data))
	return err
}

// Store borrows the shared process database; it does not own its lifetime.
type Store struct {
	db  *appstore.DB
	now func() time.Time
}

// NewStore binds bounded history queries to the existing datastore.
func NewStore(db *appstore.DB) *Store { return &Store{db: db, now: time.Now} }

// Append persists an idempotent batch and removes events older than retention.
func (s *Store) Append(ctx context.Context, events []Event) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, event := range events {
			if err := AppendTx(ctx, tx, event); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM access_audit WHERE occurred_at < ?`, s.cutoff())
		return err
	})
}

// Page is a bounded descending history window with stable sequence pagination.
type Page struct {
	Events         []Event `json:"events"`
	NewestSequence int64   `json:"newest_sequence"`
	OldestSequence int64   `json:"oldest_sequence"`
	NextBefore     int64   `json:"next_before"`
	HasMore        bool    `json:"has_more"`
	RetentionDays  int     `json:"retention_days"`
	Limit          int     `json:"limit"`
	RowLimit       int     `json:"row_limit"`
}

func (s *Store) cutoff() int64 { return s.now().Add(-RetentionDays * 24 * time.Hour).UnixMilli() }

// Prune removes expired events even while the application is otherwise idle.
func (s *Store) Prune(ctx context.Context) error { return s.Append(ctx, nil) }

// Page reads a coherent bounded batch, then releases its connection before any
// HTTP/socket write. Sequence pagination remains valid as newer events arrive.
func (s *Store) Page(ctx context.Context, before int64) (Page, error) {
	page := Page{Events: []Event{}, RetentionDays: RetentionDays, Limit: PageSize, RowLimit: appstore.AuditRowLimit}
	if before < 0 {
		return page, errors.New("invalid audit cursor")
	}
	tx, err := s.db.SQL().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, err
	}
	defer func() { _ = tx.Rollback() }()
	cutoff := s.cutoff()
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(seq),0), coalesce(min(seq),0) FROM access_audit WHERE occurred_at >= ?`, cutoff).Scan(&page.NewestSequence, &page.OldestSequence); err != nil {
		return page, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT seq, document FROM access_audit WHERE occurred_at >= ? AND (? = 0 OR seq < ?) ORDER BY seq DESC LIMIT ?`, cutoff, before, before, PageSize+1)
	if err != nil {
		return page, err
	}
	page.Events, err = readPageRows(rows)
	if err != nil {
		return page, err
	}
	if len(page.Events) > PageSize {
		page.HasMore = true
		page.Events = page.Events[:PageSize]
	}
	if len(page.Events) > 0 {
		page.NextBefore = page.Events[len(page.Events)-1].Sequence
	}
	if err := tx.Commit(); err != nil {
		return page, err
	}
	return page, nil
}

func readPageRows(rows *sql.Rows) ([]Event, error) {
	defer func() { _ = rows.Close() }()
	events := []Event{}
	for rows.Next() {
		var sequence int64
		var document string
		if err := rows.Scan(&sequence, &document); err != nil {
			return nil, err
		}
		var event Event
		if err := json.Unmarshal([]byte(document), &event); err != nil || event.ID == "" || event.OccurredAt == 0 || event.Actor.Type == "" {
			return nil, errors.New("invalid stored audit event")
		}
		event, _, err := prepare(event)
		if err != nil {
			return nil, err
		}
		event.Sequence = sequence
		events = append(events, event)
	}
	readErr, closeErr := rows.Err(), rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return events, nil
}
