package httpapi

import (
	"context"
	"errors"
	"sync"
	"time"
)

const stopWaiterLimit = 16
const stopReplyWait = 5 * time.Second

var errStopConfirmationPending = errors.New("stop requested; device confirmation is still pending")

type stopBatch struct {
	done    chan struct{}
	outcome emergencyStopResult
	err     error
}

// stopAdmissionRuntime shares only an overlapping HTTP Stop operation. It
// never caches a completed Stop and never runs background motion work.
type stopAdmissionRuntime struct {
	mu              sync.Mutex
	active          *stopBatch
	waiting         int
	shared, pending uint64
}

type stopAdmission struct {
	runtime         *stopAdmissionRuntime
	batch           *stopBatch
	leader, waiting bool
}

func (s *stopAdmissionRuntime) enter() *stopAdmission {
	s.mu.Lock()
	defer s.mu.Unlock()
	request := &stopAdmission{runtime: s, batch: s.active}
	if request.batch == nil {
		request.batch = &stopBatch{done: make(chan struct{})}
		request.leader = true
		s.active = request.batch
		return request
	}
	s.shared++
	if s.waiting < stopWaiterLimit {
		request.waiting = true
		s.waiting++
	} else {
		s.pending++
	}
	return request
}

func (r *stopAdmission) await(ctx context.Context) (emergencyStopResult, error) {
	if !r.waiting {
		return emergencyStopResult{pending: true}, errStopConfirmationPending
	}
	timer := time.NewTimer(stopReplyWait)
	defer timer.Stop()
	select {
	case <-r.batch.done:
		return r.batch.outcome, r.batch.err
	case <-ctx.Done():
	case <-timer.C:
	}
	r.runtime.mu.Lock()
	r.runtime.pending++
	r.runtime.mu.Unlock()
	return emergencyStopResult{pending: true}, errStopConfirmationPending
}

// complete runs before releasing the engine lifecycle lock. A follower can
// never join an already completed Stop after a new Start could reach hardware.
func (r *stopAdmission) complete(outcome emergencyStopResult, err error) {
	s := r.runtime
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != r.batch {
		return
	}
	r.batch.outcome, r.batch.err = outcome, err
	s.active = nil
	close(r.batch.done)
}

func (r *stopAdmission) release() {
	if r.leader {
		// Panic/early-exit protection; successful dispatch already completed
		// under the engine lock and cannot be overwritten here.
		r.complete(emergencyStopResult{pending: true}, errStopConfirmationPending)
		return
	}
	if r.waiting {
		r.runtime.mu.Lock()
		r.runtime.waiting--
		r.runtime.mu.Unlock()
	}
}

func (s *stopAdmissionRuntime) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"active": s.active != nil, "waiting": s.waiting, "waiter_limit": stopWaiterLimit,
		"shared_since_startup": s.shared, "pending_replies_since_startup": s.pending}
}
