package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

func TestSummaryCacheRespectsCanceledWaitAndConcurrentCommit(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var cache summaryCache[int]
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx := t.Context()
	go func() {
		_, err := cache.read(ctx, db, func(context.Context) (int, error) { close(started); <-release; return 1, nil })
		done <- err
	}()
	<-started
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = cache.read(waitCtx, db, func(context.Context) (int, error) { return 0, errors.New("duplicate read") })
	writeErr := db.WithTx(ctx, func(*sql.Tx) error { return nil })
	close(release)
	loadErr := <-done
	if !errors.Is(err, context.DeadlineExceeded) || writeErr != nil || loadErr != nil {
		t.Fatalf("wait=%v commit=%v load=%v", err, writeErr, loadErr)
	}
	reads := 0
	for range 2 {
		value, readErr := cache.read(ctx, db, func(context.Context) (int, error) { reads++; return 2, nil })
		if readErr != nil || value != 2 {
			t.Fatalf("post-commit summary = %d, %v", value, readErr)
		}
	}
	if reads != 1 {
		t.Fatalf("reads after commit = %d, want one", reads)
	}
}
