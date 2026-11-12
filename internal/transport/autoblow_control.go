package transport

import (
	"context"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// command sends one request on the lane, unless Stop or a newer stream has
// replaced generation since it was composed. Zero sends regardless.
func (b *autoblowBase) command(generation uint64, timeout time.Duration, send autoblowSend) error {
	b.laneMu.Lock()
	defer b.laneMu.Unlock()
	return b.commandLocked(generation, timeout, send)
}

func (b *autoblowBase) commandLocked(generation uint64, timeout time.Duration, send autoblowSend) error {
	return b.commandBoundLocked(nil, generation, timeout, send)
}

func (b *autoblowBase) commandBound(client *autoblow.Client, generation uint64, timeout time.Duration, send autoblowSend) error {
	b.laneMu.Lock()
	defer b.laneMu.Unlock()
	return b.commandBoundLocked(client, generation, timeout, send)
}

func (b *autoblowBase) commandBoundLocked(expected *autoblow.Client, generation uint64, timeout time.Duration, send autoblowSend) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	b.mu.Lock()
	client := b.client
	if client == nil {
		b.mu.Unlock()
		return errAutoblowNotConnected
	}
	if expected != nil && client != expected {
		b.mu.Unlock()
		return errAutoblowSessionEnded
	}
	if generation != 0 && (b.generation != generation || b.stopHolds > 0 || b.state == "stale") {
		b.mu.Unlock()
		return errAutoblowSuperseded
	}
	b.inFlight = cancel
	b.mu.Unlock()
	err := send(ctx, client)
	b.mu.Lock()
	b.inFlight = nil
	b.requests++
	switch {
	case err == nil:
	case errors.Is(err, autoblow.ErrRateLimited):
		b.rateLimited++
		b.failed++
	default:
		b.failed++
	}
	if err != nil && b.client == client && (generation == 0 || generation == b.generation) {
		b.lastError = err.Error()
	}
	lost := err != nil && b.client == client && lostConnection(err)
	b.mu.Unlock()
	if lost {
		go b.failConnection(client, err)
	}
	return err
}

// lostConnection is a failure that ends the connection: the device left
// online mode, or Autoblow stopped accepting the token.
func lostConnection(err error) bool {
	return errors.Is(err, autoblow.ErrOffline) || errors.Is(err, autoblow.ErrUnauthorized)
}

// stopDevice invalidates queued commands and sends stop. A request already
// on the wire gets autoblowStopGrace to finish; after that it is canceled,
// and because the cloud may still apply it after Stop, Stop is repeated.
func (b *autoblowBase) stopDevice(stop autoblowSend) error {
	b.mu.Lock()
	admission := b.admitStopLocked()
	b.mu.Unlock()
	return b.finishStop(admission, stop)
}

type autoblowStopAdmission struct {
	client     *autoblow.Client
	loops      *autoblowLoops
	generation uint64
	stale      bool
}

func (b *autoblowBase) admitStopLocked() autoblowStopAdmission {
	b.generation++
	b.stopHolds++
	return autoblowStopAdmission{client: b.client, loops: b.loops, generation: b.generation, stale: b.client == nil && b.state == "stale"}
}

func (b *autoblowBase) finishStop(admission autoblowStopAdmission, stop autoblowSend) error {
	defer func() {
		b.mu.Lock()
		b.stopHolds--
		b.mu.Unlock()
	}()
	if admission.client == nil {
		if admission.stale {
			return errAutoblowStale
		}
		return nil
	}
	canceled := false
	if !b.lockLaneWithin(autoblowStopGrace) {
		b.mu.Lock()
		cancel := b.inFlight
		b.mu.Unlock()
		if cancel != nil {
			cancel()
			canceled = true
		}
		b.laneMu.Lock()
	}
	err := b.commandBoundLocked(admission.client, 0, autoblowStopTimeout, stop)
	b.laneMu.Unlock()
	if err != nil {
		b.mu.Lock()
		if b.client == admission.client {
			b.lastError = err.Error()
			b.state = "stale"
		}
		b.mu.Unlock()
	}
	if canceled {
		b.confirmStopLater(admission, stop)
	}
	return err
}

func (b *autoblowBase) lockLaneWithin(wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if b.laneMu.TryLock() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// confirmStopLater repeats Stop once, unless a newer Play or Stop came first.
func (b *autoblowBase) confirmStopLater(admission autoblowStopAdmission, stop autoblowSend) {
	b.mu.Lock()
	loops, generation := admission.loops, admission.generation
	if loops == nil || loops != b.loops || b.client != admission.client || b.generation != generation {
		b.mu.Unlock()
		return
	}
	loops.done.Add(1)
	b.mu.Unlock()
	go func() {
		defer loops.done.Done()
		timer := time.NewTimer(autoblowStopConfirm)
		defer timer.Stop()
		select {
		case <-loops.stop:
		case <-timer.C:
			_ = b.haltGeneration(loops, generation, stop)
		}
	}()
}

// endTestLater stops a test after autoblowTestDuration unless a Stop or a
// newer stream came first.
func (b *autoblowBase) endTestLater(loops *autoblowLoops, generation uint64, stop autoblowSend) {
	b.mu.Lock()
	if loops == nil || loops != b.loops || b.client == nil || b.generation != generation {
		b.mu.Unlock()
		return
	}
	loops.done.Add(1)
	b.mu.Unlock()
	go func() {
		defer loops.done.Done()
		timer := time.NewTimer(autoblowTestDuration)
		defer timer.Stop()
		select {
		case <-loops.stop:
			return
		case <-timer.C:
		}
		err := b.haltGeneration(loops, generation, stop)
		b.mu.Lock()
		if b.generation == generation {
			b.testing = false
			if err != nil {
				b.lastError = err.Error()
				b.state = "stale"
			}
		}
		b.mu.Unlock()
	}()
}

// Cleanup may run in a failed state, but never through a newer connection or
// after a fresh Play. Hold new playback admission until the request finishes.
func (b *autoblowBase) haltGeneration(loops *autoblowLoops, generation uint64, stop autoblowSend) error {
	b.laneMu.Lock()
	defer b.laneMu.Unlock()
	b.mu.Lock()
	if loops == nil || b.loops != loops || b.generation != generation || b.client == nil {
		b.mu.Unlock()
		return errAutoblowSuperseded
	}
	client := b.client
	b.stopHolds++
	b.mu.Unlock()
	defer func() { b.mu.Lock(); b.stopHolds--; b.mu.Unlock() }()
	err := b.commandBoundLocked(client, 0, autoblowStopTimeout, stop)
	if err != nil {
		b.mu.Lock()
		if b.client == client && b.generation == generation {
			b.state = "stale"
		}
		b.mu.Unlock()
		go b.failConnection(client, err)
	}
	return err
}
