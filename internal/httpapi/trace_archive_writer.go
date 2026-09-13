package httpapi

import (
	"context"
	"sync"
	"time"
)

// traceArchiveWriter retains only the latest stopped run. Its immutable document
// is immediately readable; the single worker performs persistence independently
// of Stop. At most one pending and one in-flight document can await storage.
type traceArchiveWriter struct {
	mu       sync.Mutex
	latest   []byte
	pending  []byte
	sequence uint64
	closed   bool
	wake     chan struct{}
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	write    func(context.Context, []byte)
}

func newTraceArchiveWriter(write func(context.Context, []byte)) *traceArchiveWriter {
	ctx, cancel := context.WithCancel(context.Background())
	w := &traceArchiveWriter{wake: make(chan struct{}, 1), done: make(chan struct{}), ctx: ctx, cancel: cancel, write: write}
	go w.run()
	return w
}

func (w *traceArchiveWriter) record(sequence uint64, document []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || sequence <= w.sequence {
		return
	}
	w.sequence, w.latest, w.pending = sequence, document, document
	w.signal()
}

func (w *traceArchiveWriter) snapshot() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.latest // Published documents are immutable.
}

func (w *traceArchiveWriter) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *traceArchiveWriter) run() {
	defer close(w.done)
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
			w.mu.Lock()
			document, closed := w.pending, w.closed
			w.pending = nil
			w.mu.Unlock()
			if document != nil {
				ctx, cancel := context.WithTimeout(w.ctx, 2*time.Second)
				w.write(ctx, document)
				cancel()
			}
			if closed {
				return
			}
		}
	}
}

func (w *traceArchiveWriter) close() {
	w.mu.Lock()
	w.closed = true
	w.signal()
	w.mu.Unlock()
	timer := time.AfterFunc(2*time.Second, w.cancel)
	<-w.done
	timer.Stop()
	w.cancel()
}
