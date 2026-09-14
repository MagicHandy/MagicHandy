package httpapi

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTraceArchiveCoalescesPendingRunsAndFlushesOnClose(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var mu sync.Mutex
	var writes []string
	w := newTraceArchiveWriter(func(ctx context.Context, document []byte) {
		if string(document) == "first" {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return
			}
		}
		mu.Lock()
		writes = append(writes, string(document))
		mu.Unlock()
	})
	t.Cleanup(w.close)
	w.record(1, []byte("first"))
	<-entered
	for sequence := uint64(2); sequence < 100; sequence++ {
		w.record(sequence, []byte("superseded"))
	}
	w.record(100, []byte("latest"))
	w.record(50, []byte("late older callback"))
	if string(w.snapshot()) != "latest" {
		t.Fatal("latest trace was unavailable while storage was blocked")
	}
	once.Do(func() { close(release) })
	w.close()
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 2 || writes[0] != "first" || writes[1] != "latest" {
		t.Fatalf("unbounded or reordered trace writes: %v", writes)
	}
}

func TestTraceArchiveCloseCancelsStorage(t *testing.T) {
	entered := make(chan struct{})
	w := newTraceArchiveWriter(func(ctx context.Context, _ []byte) { close(entered); <-ctx.Done() })
	w.record(1, []byte("trace"))
	<-entered
	done := make(chan struct{})
	go func() { w.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("trace worker survived its shutdown bound")
	}
	w.record(2, []byte("after shutdown"))
	if string(w.snapshot()) != "trace" {
		t.Fatal("closed writer accepted more work")
	}
}
