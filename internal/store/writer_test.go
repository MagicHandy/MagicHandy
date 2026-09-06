package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func holdWriter(t *testing.T, db *DB) func() {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- db.WithTx(t.Context(), func(*sql.Tx) error { close(entered); <-release; return nil })
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("hold writer: %v", err)
	}
	finish := sync.OnceFunc(func() {
		close(release)
		if err := <-done; err != nil {
			t.Errorf("held transaction: %v", err)
		}
	})
	t.Cleanup(finish)
	return finish
}

func TestWriterAdmissionHonorsCancellationWhileBusy(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			db, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			release := holdWriter(t, db)
			defer release()
			ctx, cancel := context.WithCancel(t.Context())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
				want = context.DeadlineExceeded
			}
			defer cancel()
			var called atomic.Bool
			done := make(chan error, 1)
			go func() { done <- db.WithTx(ctx, func(*sql.Tx) error { called.Store(true); return nil }) }()
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("admission = %v, want %v", err, want)
				}
			case <-time.After(time.Second):
				release()
				<-done
				t.Fatal("canceled admission waited for the current writer")
			}
			if called.Load() {
				t.Fatal("canceled callback ran")
			}
		})
	}
}

func TestWriterSerializesConcurrentTransactionsAndReleasesFailures(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var active atomic.Int32
	var overlap atomic.Bool
	var group sync.WaitGroup
	failure := errors.New("rollback fixture")
	for i := range 24 {
		group.Go(func() {
			err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				if active.Add(1) != 1 {
					overlap.Store(true)
				}
				defer active.Add(-1)
				_, err := tx.ExecContext(t.Context(), "INSERT INTO app_kv(key,value,updated_at) VALUES(?,?,?)", i, "value", "now")
				if err != nil {
					return err
				}
				if i%2 == 0 {
					return failure
				}
				return nil
			})
			if err != nil && !errors.Is(err, failure) {
				t.Errorf("transaction: %v", err)
			}
		})
	}
	group.Wait()
	if overlap.Load() {
		t.Fatal("writer callbacks overlapped")
	}
	var count int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM app_kv").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 12 {
		t.Fatalf("committed rows = %d, want 12", count)
	}
}
