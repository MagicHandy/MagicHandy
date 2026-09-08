// Package workerhost shares the bounded job lifecycle used by provider workers.
// It owns cancellation only; model APIs, credentials and readiness stay in the
// provider adapters, outside the pure-Go core.
package workerhost

import (
	"context"
	"sync"
)

type job struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Jobs includes queued work, so unload/reload cannot revive an old queued job.
// Its zero value is usable. One active job and eight queued jobs are retained.
type Jobs struct {
	mu   sync.Mutex
	jobs map[string]job
}

// Track reserves a bounded slot before the worker queues a new request.
func (j *Jobs) Track(id string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if id == "" || len(j.jobs) >= 9 {
		return false
	}
	if _, exists := j.jobs[id]; exists {
		return false
	}
	if j.jobs == nil {
		j.jobs = make(map[string]job)
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.jobs[id] = job{ctx, cancel}
	return true
}

// Context returns the job lifetime; an unknown job is already canceled.
func (j *Jobs) Context(id string) context.Context {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job, ok := j.jobs[id]; ok {
		return job.ctx
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// Canceled reports whether cancellation invalidated a tracked job.
func (j *Jobs) Canceled(id string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	job, ok := j.jobs[id]
	return ok && job.ctx.Err() != nil
}

// Cancel invalidates a known job without retaining unknown request IDs.
func (j *Jobs) Cancel(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job, ok := j.jobs[id]; ok {
		job.cancel()
	}
}

// CancelAll invalidates both queued and currently executing jobs.
func (j *Jobs) CancelAll() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, job := range j.jobs {
		job.cancel()
	}
}

// Finish releases a completed job's slot and cancellation resources.
func (j *Jobs) Finish(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job, ok := j.jobs[id]; ok {
		job.cancel()
		delete(j.jobs, id)
	}
}
