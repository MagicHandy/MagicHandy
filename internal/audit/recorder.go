package audit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// QueueLimit is the maximum number of waiting runtime events.
const QueueLimit = 256
const batchLimit = 64

// Recorder never puts database work on a motion/Stop caller. One bounded queue,
// one writer and short cancelable transactions limit failure and shutdown costs.
type Recorder struct {
	mu               sync.Mutex
	queue            chan Event
	closed           bool
	done             chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
	append           func(context.Context, []Event) error
	prune            func(context.Context) error
	dropped          atomic.Uint64
	failures         atomic.Uint64
	pending          atomic.Int64
	lastWrite        atomic.Int64
	available        atomic.Bool
	acknowledgedLoss uint64
}

// Status reports bounded queue occupancy and explicit loss since process start.
type Status struct {
	QueueDepth       int64  `json:"queue_depth"`
	QueueLimit       int    `json:"queue_limit"`
	Dropped          uint64 `json:"dropped_since_startup"`
	WriteFailures    uint64 `json:"write_failures_since_startup"`
	StorageAvailable bool   `json:"storage_available"`
	LastWrite        int64  `json:"last_write_ms"`
}

// NewRecorder starts one worker; its owner must close it before the datastore.
func NewRecorder(store *Store) *Recorder { return newRecorder(store.Append, store.Prune) }

func newRecorder(appendEvents func(context.Context, []Event) error, prune func(context.Context) error) *Recorder {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Recorder{queue: make(chan Event, QueueLimit), done: make(chan struct{}), ctx: ctx, cancel: cancel, append: appendEvents, prune: prune}
	go r.run()
	return r
}

// Record validates and enqueues without waiting for storage; false means no enqueue.
func (r *Recorder) Record(event Event) bool {
	event, _, err := prepare(event)
	if err != nil {
		r.dropped.Add(1)
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	select {
	case r.queue <- event:
		return true
	default:
		r.dropped.Add(1)
		return false
	}
}

// Status reads memory-only counters without touching storage.
func (r *Recorder) Status() Status {
	return Status{QueueDepth: int64(len(r.queue)) + r.pending.Load(), QueueLimit: QueueLimit + batchLimit,
		Dropped: r.dropped.Load(), WriteFailures: r.failures.Load(), StorageAvailable: r.available.Load(), LastWrite: r.lastWrite.Load()}
}

// Close drains after motion has quiesced, for at most two seconds;
// a blocked writer is canceled rather than leaking into the next app lifetime.
func (r *Recorder) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	timer := time.AfterFunc(2*time.Second, r.cancel)
	<-r.done
	timer.Stop()
	r.cancel()
}

func (r *Recorder) run() {
	defer close(r.done)
	gapCheck := time.NewTicker(time.Second)
	defer gapCheck.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case event, ok := <-r.queue:
			if !ok {
				r.write(nil)
				return
			}
			batch := r.collect(event)
			r.write(batch)
		case <-gapCheck.C:
			if r.dropped.Load() > r.acknowledgedLoss {
				r.write(nil)
			}
		case <-prune.C:
			ctx, cancel := context.WithTimeout(r.ctx, time.Second)
			err := r.prune(ctx)
			cancel()
			if err != nil {
				r.failures.Add(1)
				r.available.Store(false)
			}
		case <-r.ctx.Done():
			r.dropped.Add(uint64(len(r.queue)))
			return
		}
	}
}

func (r *Recorder) collect(first Event) []Event {
	batch := make([]Event, 0, batchLimit+1)
	batch = append(batch, first)
	for len(batch) < batchLimit {
		select {
		case event, ok := <-r.queue:
			if !ok {
				return batch
			}
			batch = append(batch, event)
		default:
			return batch
		}
	}
	return batch
}

func (r *Recorder) write(batch []Event) {
	count := len(batch)
	loss := r.dropped.Load()
	if loss > r.acknowledgedLoss {
		gap, _, err := prepare(Event{Kind: HistoryGap, Outcome: "incomplete", Count: loss - r.acknowledgedLoss})
		if err == nil {
			batch = append(batch, gap)
		}
	}
	if len(batch) == 0 {
		return
	}
	r.pending.Store(int64(count))
	defer r.pending.Store(0)
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(r.ctx, 500*time.Millisecond)
		err := r.append(ctx, batch)
		cancel()
		if err == nil {
			r.acknowledgedLoss = loss
			r.available.Store(true)
			r.lastWrite.Store(time.Now().UnixMilli())
			return
		}
		r.failures.Add(1)
		r.available.Store(false)
	}
	r.dropped.Add(uint64(count))
}
