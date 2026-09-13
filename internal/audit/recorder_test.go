package audit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecorderNeverWaitsForStorageAndReportsOverflow(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var persisted []Event
	recorder := newRecorder(func(ctx context.Context, batch []Event) error {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		persisted = append(persisted, batch...)
		return nil
	}, func(context.Context) error { return nil })
	t.Cleanup(recorder.Close)
	recorder.Record(Event{Kind: StopFinished, Outcome: "success"})
	<-started
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := 0; i < QueueLimit+20; i++ {
			recorder.Record(Event{Kind: StopFinished, Outcome: "unconfirmed"})
		}
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Stop event waited for blocked storage")
	}
	if recorder.Status().Dropped == 0 || recorder.Status().QueueDepth > QueueLimit+batchLimit {
		t.Fatal("overflow was unbounded or unreported")
	}
	close(release)
	recorder.Close()
	var gaps uint64
	for _, event := range persisted {
		if event.Kind == HistoryGap {
			gaps += event.Count
		}
	}
	if gaps != recorder.Status().Dropped {
		t.Fatalf("durable gap=%d dropped=%d", gaps, recorder.Status().Dropped)
	}
	if recorder.Record(Event{Kind: StopFinished, Outcome: "success"}) {
		t.Fatal("closed writer accepted later work")
	}
}

func TestRecorderRetryDoesNotDuplicateCommittedEvents(t *testing.T) {
	s, _ := testStore(t)
	var calls atomic.Int32
	r := newRecorder(func(ctx context.Context, batch []Event) error {
		if err := s.Append(ctx, batch); err != nil {
			return err
		}
		if calls.Add(1) == 1 {
			return errors.New("simulated ambiguous commit acknowledgement")
		}
		return nil
	}, s.Prune)
	t.Cleanup(r.Close)
	r.Record(Event{Kind: CommandFinished, Outcome: "success"})
	r.Close()
	page, err := s.Page(t.Context(), 0)
	if err != nil || len(page.Events) != 1 || r.Status().Dropped != 0 {
		t.Fatalf("retry duplicated or lost a committed event: %+v %v", page, err)
	}
}

func TestRecorderCloseCancelsBlockedWrites(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	r := newRecorder(func(ctx context.Context, _ []Event) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	}, func(context.Context) error { return nil })
	t.Cleanup(r.Close)
	r.Record(Event{Kind: StopFinished, Outcome: "unconfirmed"})
	<-entered
	for i := 0; i < QueueLimit; i++ {
		r.Record(Event{Kind: StopFinished, Outcome: "unconfirmed"})
	}
	done := make(chan struct{})
	go func() { r.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("audit writer survived shutdown")
	}
	if r.Status().WriteFailures == 0 || r.Status().StorageAvailable {
		t.Fatal("storage failure was hidden")
	}
}
