package patterns

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestFeedbackCancellationWhileWaitingForWriterPreservesHistoryAndWeight(t *testing.T) {
	for _, undo := range []bool{false, true} {
		name := "apply"
		if undo {
			name = "undo"
		}
		t.Run(name, func(t *testing.T) {
			library, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = library.Close() })
			id := string(motion.PatternHardAndRegular)
			var feedbackID int64
			if undo {
				feedback, _, err := library.ApplyFeedback(id, 1)
				if err != nil {
					t.Fatal(err)
				}
				feedbackID = feedback.ID
			}
			before, err := library.Pattern(id)
			if err != nil {
				t.Fatal(err)
			}
			release := holdFeedbackWriter(t, library)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if undo {
				_, _, err = library.UndoFeedbackContext(ctx, feedbackID)
			} else {
				_, _, err = library.ApplyFeedbackContext(ctx, id, -1)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("queued %s: %v", name, err)
			}
			release()
			after, err := library.Pattern(id)
			if err != nil || before.Weight != after.Weight || before.Enabled != after.Enabled || before.UpdatedAt != after.UpdatedAt {
				t.Fatalf("canceled %s changed pattern: %v", name, err)
			}
			history, err := library.FeedbackHistory(100)
			if err != nil || (!undo && len(history) != 0) || (undo && (len(history) != 1 || history[0].Reverted)) {
				t.Fatalf("canceled %s changed feedback history: %v", name, err)
			}
		})
	}
}

func holdFeedbackWriter(t *testing.T, library *Library) func() {
	t.Helper()
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- library.db.WithTx(context.Background(), func(*sql.Tx) error { close(locked); <-release; return nil })
	}()
	<-locked
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(release)
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(finish)
	return finish
}
