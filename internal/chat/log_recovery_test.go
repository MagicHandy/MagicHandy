package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

func recoveryLog(t *testing.T) (*MessageLog, string) {
	t.Helper()
	l, err := OpenMessageLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	id, err := l.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	return l, id
}

func recoveryPage(t *testing.T, l *MessageLog, id string, after int64, limit int) MessagePage {
	t.Helper()
	page, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, ClientID: "reader", AfterRevision: &after, Limit: limit})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func appendRecoveryMessage(t *testing.T, l *MessageLog, content string) int64 {
	t.Helper()
	seq, err := l.Append(MessageRoleUser, content, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

func TestRecoveryFindsReplyCommittedBelowDisplayHead(t *testing.T) {
	l, id := recoveryLog(t)
	appendRecoveryMessage(t, l, "first")
	pending, err := l.AppendPendingAssistantTo(id, "late reply", nil)
	if err != nil {
		t.Fatal(err)
	}
	latest := appendRecoveryMessage(t, l, "newer display sequence")
	before := recoveryPage(t, l, id, 0, 0)
	if before.LatestSeq != latest || len(before.Messages) != 2 || before.Revision != 2 {
		t.Fatalf("initial snapshot: %+v", before)
	}
	if _, err := l.AdvanceReadCursorContext(t.Context(), "reader", id, latest, &before.NextRevision); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitPending(pending); err != nil {
		t.Fatal(err)
	}
	after := recoveryPage(t, l, id, before.NextRevision, 0)
	if after.Reset || after.LatestSeq != latest || after.Revision != 3 || len(after.Messages) != 1 || after.Messages[0].Seq != pending || after.NextRevision != 3 {
		t.Fatalf("late committed reply missing: %+v", after)
	}
	if after.Cursor != latest || after.CursorRevision != before.NextRevision {
		t.Fatal("display and recovery cursors were conflated")
	}
	path := l.db.DataDir()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenMessageLog(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recovered := recoveryPage(t, reopened, id, before.NextRevision, 0)
	if len(recovered.Messages) != 1 || recovered.Messages[0].Seq != pending || recovered.CursorRevision != before.NextRevision {
		t.Fatalf("restart lost recovery: %+v", recovered)
	}
}

func TestRecoveryPagesAdvanceOnlyThroughDeliveredChanges(t *testing.T) {
	l, id := recoveryLog(t)
	for index := 0; index < 4; index++ {
		appendRecoveryMessage(t, l, fmt.Sprintf("message %d", index))
	}
	for revision := int64(1); revision < 4; revision++ {
		page := recoveryPage(t, l, id, revision, 1)
		if len(page.Messages) != 1 || page.NextRevision != revision+1 || page.Revision != 4 || page.HasMore != (revision < 3) {
			t.Fatalf("partial page skipped a commit: %+v", page)
		}
	}
	if err := l.Delete(2); err != nil {
		t.Fatal(err)
	}
	reset := recoveryPage(t, l, id, 3, 1)
	if !reset.Reset || reset.HasMore || reset.NextRevision != 5 || len(reset.Messages) != 3 {
		t.Fatalf("deletion did not give a complete reset: %+v", reset)
	}
	future := recoveryPage(t, l, id, 100, 0)
	if !future.Reset || future.NextRevision != 5 {
		t.Fatalf("future cursor was not reset: %+v", future)
	}
}

func TestRecoveryReportsRetainedWindowGap(t *testing.T) {
	l, id := recoveryLog(t)
	for index := 0; index < MessageLogCap+3; index++ {
		appendRecoveryMessage(t, l, fmt.Sprintf("retained message %d", index))
	}
	page := recoveryPage(t, l, id, 1, 1)
	if !page.Reset || !page.HistoryGap || page.HasMore || len(page.Messages) != MessageLogCap || page.FirstSeq != 4 || page.NextRevision != MessageLogCap+3 {
		t.Fatalf("retention loss was hidden: %+v", page)
	}
	current := recoveryPage(t, l, id, page.NextRevision, 0)
	if current.Reset || current.HistoryGap || len(current.Messages) != 0 {
		t.Fatalf("current reader was unnecessarily reset: %+v", current)
	}
}

func TestRecoverySnapshotIsConsistentDuringConcurrentCommits(t *testing.T) {
	l, id := recoveryLog(t)
	done := make(chan error, 1)
	go func() {
		for index := 0; index < 100; index++ {
			if _, err := l.Append(MessageRoleUser, fmt.Sprintf("concurrent %d", index), "fixture"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for index := 0; index < 100; index++ {
		page := recoveryPage(t, l, id, 0, 0)
		var maxSeq, maxRevision int64
		for _, message := range page.Messages {
			maxSeq = max(maxSeq, message.Seq)
			maxRevision = max(maxRevision, message.Revision)
		}
		if page.LatestSeq != maxSeq || page.Revision != maxRevision || page.NextRevision != maxRevision {
			t.Fatalf("snapshot mixed commits: %+v", page)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCursorWriteCancelsBeforeQueuedTransactionApplies(t *testing.T) {
	l, id := recoveryLog(t)
	seq := appendRecoveryMessage(t, l, "cursor cancellation")
	locked, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		writerDone <- l.db.WithTx(context.Background(), func(*sql.Tx) error { close(locked); <-release; return nil })
	}()
	<-locked
	t.Cleanup(func() {
		close(release)
		if err := <-writerDone; err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := l.AdvanceCursorSessionContext(ctx, "reader", id, seq); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued cursor write: %v", err)
	}
	cursor, err := l.CursorSession("reader", id)
	if err != nil || cursor != 0 {
		t.Fatalf("canceled cursor changed: %d, %v", cursor, err)
	}
}

func TestCursorRetentionAndCardinalityAreBounded(t *testing.T) {
	l, id := recoveryLog(t)
	seq := appendRecoveryMessage(t, l, "bounded cursor bookkeeping")
	now := time.Now().UTC()
	_, err := l.db.SQL().Exec(`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value < ?)
		INSERT INTO chat_session_cursors(client_id, session_id, last_seq, last_revision, updated_at)
		SELECT 'old-' || value, ?, 1, 1, ? FROM n`, ChatCursorCap, id, now.Add(-time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.AdvanceCursorSessionContext(t.Context(), "new-reader", id, seq); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := l.db.SQL().QueryRow(`SELECT COUNT(*) FROM chat_session_cursors`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != ChatCursorCap {
		t.Fatalf("cursor count = %d", count)
	}
	if _, err := l.db.SQL().Exec(`UPDATE chat_session_cursors SET updated_at = ? WHERE client_id = 'new-reader'`, now.Add(-25*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if cursor, err := l.CursorSession("new-reader", id); err != nil || cursor != 0 {
		t.Fatalf("expired cursor = %d, %v", cursor, err)
	}
	if _, err := l.AdvanceCursorSessionContext(t.Context(), "active-reader", id, seq); err != nil {
		t.Fatal(err)
	}
	if err := l.db.SQL().QueryRow(`SELECT COUNT(*) FROM chat_session_cursors WHERE client_id = 'new-reader'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired cursor not reaped: %d, %v", count, err)
	}
}
