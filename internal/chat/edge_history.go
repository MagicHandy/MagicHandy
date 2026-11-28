package chat

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// EdgeObservation describes a button event. Return timing is the accepted
// plan, not a claim that the user waited for the countdown or that a device moved.
type EdgeObservation struct {
	PressedAt      time.Time `json:"-"`
	Active         string    `json:"currently_active,omitempty"`
	Kind           string    `json:"hold"`
	HoldingPace    int       `json:"holding_pace_percent,omitempty"`
	PlannedSeconds int       `json:"planned_hold_seconds,omitempty"`
	ReturnPace     int       `json:"planned_return_percent,omitempty"`
	ReturnStyle    string    `json:"planned_return_style,omitempty"`
}

// EdgeFeedback pairs an observed hold with the human's own later remarks.
type EdgeFeedback struct {
	Observation EdgeObservation `json:"earlier_button_press"`
	Remarks     []string        `json:"subsequent_user_remarks_oldest_first"`
}

// EdgeChatAnchor takes both values in one read before dispatch. A later tab
// switch or a quick comment while a command is in flight cannot rebind the press.
func (l *MessageLog) EdgeChatAnchor(ctx context.Context) (string, int64, error) {
	var session string
	var seq int64
	err := l.db.SQL().QueryRowContext(ctx, `SELECT active_session_id,
		COALESCE((SELECT MAX(seq) FROM messages WHERE session_id=active_session_id AND committed=1),0)
		FROM chat_workspace WHERE id='current'`).Scan(&session, &seq)
	return session, seq, err
}

// RecordEdge persists an accepted press in its originating conversation.
func (l *MessageLog) RecordEdge(ctx context.Context, session string, at time.Time, afterSeq int64, kind string, pace int) error {
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO chat_edge_holds
			(session_id,pressed_ns,after_seq,kind,holding_pace) VALUES(?,?,?,?,?)`, session, at.UnixNano(), afterSeq, kind, pace)
		return err
	})
}

// RecordEdgePlan records only the return accepted for this specific press.
func (l *MessageLog) RecordEdgePlan(ctx context.Context, session string, at time.Time, plan EdgePlan) error {
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE chat_edge_holds SET planned_seconds=?,return_pace=?,return_style=?
			WHERE session_id=? AND pressed_ns=?`, plan.HoldSeconds, plan.ResumeSpeed, plan.ResumeStyle, session, at.UnixNano())
		return err
	})
}

func (l *MessageLog) lastEdgeBefore(ctx context.Context, session string, before int64) (*EdgeObservation, int64, error) {
	var item EdgeObservation
	var anchor, pressed int64
	err := l.db.SQL().QueryRowContext(ctx, `SELECT kind,holding_pace,planned_seconds,return_pace,return_style,after_seq,pressed_ns
		FROM chat_edge_holds WHERE session_id=? AND pressed_ns<? ORDER BY pressed_ns DESC LIMIT 1`,
		session, before).Scan(&item.Kind, &item.HoldingPace, &item.PlannedSeconds, &item.ReturnPace, &item.ReturnStyle, &anchor, &pressed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	item.PressedAt = time.Unix(0, pressed)
	return &item, anchor, err
}

// LatestEdge supplies a fact only in the conversation where the press occurred.
func (l *MessageLog) LatestEdge(ctx context.Context, session string) (*EdgeObservation, error) {
	item, _, err := l.lastEdgeBefore(ctx, session, 1<<63-1)
	return item, err
}

// EdgeFeedbackBefore reads first reactions, named hold notes and latest
// corrections. Skip oversized lines whole so truncation cannot change a request.
func (l *MessageLog) EdgeFeedbackBefore(ctx context.Context, session string, press time.Time, through int64) (*EdgeFeedback, error) {
	item, after, err := l.lastEdgeBefore(ctx, session, press.UnixNano())
	if err != nil || item == nil || through <= after {
		return nil, err
	}
	rows, err := l.db.SQL().QueryContext(ctx, `WITH remarks AS (
		SELECT seq,content FROM messages WHERE session_id=? AND committed=1 AND role='user'
		AND seq>? AND seq<=? AND length(CAST(content AS BLOB)) BETWEEN 1 AND 384
		AND EXISTS (SELECT 1 FROM chat_edge_holds WHERE session_id=? AND pressed_ns=? AND after_seq=?)
	), opening AS (SELECT * FROM remarks ORDER BY seq LIMIT 2),
	named AS (SELECT * FROM remarks WHERE instr(lower(content),'edge')>0 OR instr(lower(content),'hold')>0 ORDER BY seq DESC LIMIT 2),
	latest AS (SELECT * FROM remarks ORDER BY seq DESC LIMIT 3)
	SELECT content FROM (SELECT * FROM opening UNION SELECT * FROM named UNION SELECT * FROM latest) ORDER BY seq`,
		session, after, through, session, press.UnixNano(), through)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	feedback := EdgeFeedback{Observation: *item}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		feedback.Remarks = append(feedback.Remarks, line)
	}
	if err := rows.Err(); err != nil || len(feedback.Remarks) == 0 {
		return nil, err
	}
	return &feedback, nil
}
