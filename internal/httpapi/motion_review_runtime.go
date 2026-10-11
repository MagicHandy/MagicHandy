package httpapi

import (
	"context"
	"sync"
	"time"
)

const motionReviewDeadline = 10 * time.Second

// Status contains only fixed outcome codes, never provider text or user prompts.
type motionReviewStatus struct {
	State         string `json:"state"`
	Source        string `json:"source,omitempty"`
	Model         string `json:"model,omitempty"`
	Reason        string `json:"reason,omitempty"`
	ElapsedMillis int64  `json:"elapsed_ms,omitempty"`
	Revision      uint64 `json:"revision"`
}

// One worker and one replaceable pending job. A slow or cancellation-delayed
// provider cannot create an unbounded goroutine or request backlog.
type motionReviewRuntime struct {
	mu              sync.Mutex
	wg              sync.WaitGroup
	pending         *motionReviewJob
	cancel          context.CancelFunc
	running, closed bool
	revision        uint64
	status          motionReviewStatus
}

func (r *motionReviewRuntime) snapshot() motionReviewStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *motionReviewRuntime) invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revision++
	r.pending = nil
	if r.cancel != nil {
		r.cancel()
	}
	if r.status.State == "checking" {
		r.status.State, r.status.Revision = "skipped", r.revision
	}
}

func (r *motionReviewRuntime) close() {
	r.mu.Lock()
	r.closed = true
	r.pending = nil
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	r.wg.Wait()
}

func (s *Server) submitMotionReview(job motionReviewJob) {
	r := &s.motionReview
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.revision++
	job.revision = r.revision
	r.pending = &job
	if r.cancel != nil {
		r.cancel()
	}
	r.status = motionReviewStatus{State: "checking", Source: job.input.Source, Model: job.reviewer.Model, Revision: job.revision}
	if !r.running {
		r.running = true
		r.wg.Add(1)
		go s.runMotionReviews()
	}
}

func (s *Server) runMotionReviews() {
	r := &s.motionReview
	defer r.wg.Done()
	for {
		r.mu.Lock()
		job := r.pending
		if job == nil || r.closed {
			r.running, r.cancel = false, nil
			r.mu.Unlock()
			return
		}
		r.pending = nil
		ctx, cancel := context.WithDeadline(context.Background(), job.queuedAt.Add(motionReviewDeadline))
		r.cancel = cancel
		r.mu.Unlock()
		status := s.runMotionReview(ctx, *job, cancel)
		cancel()
		r.mu.Lock()
		if r.revision == job.revision {
			status.Revision = job.revision
			r.status = status
		}
		r.mu.Unlock()
	}
}
