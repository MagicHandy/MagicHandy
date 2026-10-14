package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type playbackAligner interface{ AlignPlaybackStart(time.Time) }

// FanOut is one linked playback session. Timing and sampling follow the Handy;
// every participant must remain healthy. A fault latches until a new session is
// constructed, so reconnecting cannot silently rejoin an existing run.
type FanOut struct {
	primary          Transport
	companion        Transport
	mu               sync.Mutex
	playing          bool
	fault            error
	continueHealthy  bool
	retired          [2]bool
	warnings         string
	faultStopped     bool
	faultParticipant string
}

type fanOutWithStartup struct {
	*FanOut
	startup MotionStartupStateProvider
}

// NewFanOut creates a linked session with stop-all failure handling.
func NewFanOut(primary, companion Transport) Transport {
	return NewFanOutWithPolicy(primary, companion, false)
}

// NewFanOutWithPolicy optionally retires a failed participant during playback.
// Startup still requires both, and a retired connection cannot hot-join.
func NewFanOutWithPolicy(primary, companion Transport, continueHealthy bool) Transport {
	fanOut := &FanOut{primary: primary, companion: companion, continueHealthy: continueHealthy}
	if startup, ok := primary.(MotionStartupStateProvider); ok {
		return &fanOutWithStartup{FanOut: fanOut, startup: startup}
	}
	return fanOut
}

func (f *fanOutWithStartup) ReadMotionStartupState(ctx context.Context) (MotionStartupState, MotionStartupStateResults, error) {
	state, results, err := f.startup.ReadMotionStartupState(ctx)
	if err != nil {
		f.latch("Handy", err)
	}
	return state, results, err
}

// Stop always reaches both participants, even if one fails. Fault stops zero
// electrical outputs; an ordinary Pause retains their calibrated strength.
func (f *FanOut) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	f.mu.Lock()
	command.Fault = command.Fault || f.fault != nil && !f.faultStopped
	f.playing = false
	f.mu.Unlock()
	var companionErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		_, companionErr = f.companion.Stop(ctx, command)
	}()
	result, primaryErr := f.primary.Stop(ctx, command)
	wait.Wait()
	err := errors.Join(labelError("Handy", primaryErr), labelError("Coyote", companionErr))
	if err != nil {
		f.latch("linked Stop", err)
		result.OK = false
		result.Error = err.Error()
	}
	if err == nil && command.Fault {
		f.mu.Lock()
		f.faultStopped = true
		f.mu.Unlock()
	}
	return result, err
}

// SetStrokeWindow applies the shared stroke window to each active participant.
func (f *FanOut) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return f.dispatch(ctx, func(t Transport) (CommandResult, error) { return t.SetStrokeWindow(ctx, command) })
}

// AppendPoints sends the shared engine's points to each active participant.
func (f *FanOut) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	return f.dispatch(ctx, func(t Transport) (CommandResult, error) { return t.AppendPoints(ctx, command) })
}

// Play starts the prepared participants and aligns the companion's playback.
func (f *FanOut) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	result, err := f.dispatch(ctx, func(t Transport) (CommandResult, error) { return t.Play(ctx, command) })
	if err != nil {
		return result, err
	}
	if aligner, ok := f.companion.(playbackAligner); ok {
		if provider, ok := f.primary.(PlaybackStartTimeProvider); ok {
			aligner.AlignPlaybackStart(provider.PlaybackStartTime())
		}
	}
	f.mu.Lock()
	f.playing = true
	f.mu.Unlock()
	return result, nil
}

func (f *FanOut) dispatch(ctx context.Context, send func(Transport) (CommandResult, error)) (CommandResult, error) {
	f.mu.Lock()
	fault, retired := f.fault, f.retired
	f.mu.Unlock()
	if fault != nil {
		return CommandResult{}, fault
	}
	var result CommandResult
	for index, participant := range []Transport{f.primary, f.companion} {
		if retired[index] {
			continue
		}
		next, err := send(participant)
		if err != nil {
			if f.retire(ctx, index, err) {
				continue
			}
			return next, f.failedCommand(ctx, participantName(index), err)
		}
		if !result.OK {
			result = next
		}
	}
	return result, nil
}

func participantName(index int) string {
	if index == 0 {
		return "Handy"
	}
	return "Coyote"
}

// retire only applies to an established run with the explicit continue policy.
func (f *FanOut) retire(ctx context.Context, index int, cause error) bool {
	f.mu.Lock()
	if !f.playing || !f.continueHealthy || f.retired[1-index] {
		f.mu.Unlock()
		return false
	}
	already := f.retired[index]
	f.retired[index] = true
	f.warnings = participantName(index) + " failed; continuing on the other device. Reconnect and start again to rejoin."
	f.mu.Unlock()
	if already {
		return true
	}
	participant := f.primary
	if index == 1 {
		participant = f.companion
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := participant.Stop(stopCtx, StopCommand{Reason: "linked_participant_failed", Fault: true})
	if err != nil {
		f.mu.Lock()
		f.warnings += " Stop could not be confirmed."
		f.mu.Unlock()
	}
	_ = cause // Never expose raw transport errors in diagnostic summaries.
	return true
}

// Stop the healthy participant as soon as a command fails; the engine then
// records/reconciles its own stopped state through its normal recovery path.
func (f *FanOut) failedCommand(ctx context.Context, participant string, cause error) error {
	f.latch(participant, cause)
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, stopErr := f.Stop(stopCtx, StopCommand{Reason: "linked_participant_failed", Fault: true})
	return errors.Join(labelError(participant, cause), stopErr)
}

func labelError(participant string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", participant, err)
}

func (f *FanOut) latch(participant string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fault == nil {
		f.fault = labelError(participant, err)
		f.faultParticipant = participant
	}
}

// Diagnostics is sampled by the engine's existing health loop. It has no
// independent dispatch goroutine and cannot resume a failed session.
func (f *FanOut) Diagnostics() TransportDiagnostics {
	primary, companion := f.primary.Diagnostics(), f.companion.Diagnostics()
	f.mu.Lock()
	playing, fault, retired := f.playing, f.fault, f.retired
	f.mu.Unlock()
	if fault == nil && playing && companion.PlaybackState != "stopping" {
		for index, state := range []TransportDiagnostics{primary, companion} {
			if retired[index] {
				continue
			}
			if !state.Connected || linkedPlaybackUnhealthy(state.PlaybackState) {
				cause := fmt.Errorf("playback lost (%s); linked session stopped", state.PlaybackState)
				if !f.retire(context.Background(), index, cause) {
					f.latch(participantName(index), cause)
				}
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	result := primary
	if f.retired[0] {
		result = companion
	}
	result.Connected = (primary.Connected || f.retired[0]) && (companion.Connected || f.retired[1])
	if f.fault != nil {
		result.PlaybackState = "stale"
		result.LastError = f.faultParticipant + " failed; linked session stopped. Start a new session."
	} else if f.warnings != "" {
		result.LastError = f.warnings
	}
	return result
}

func linkedPlaybackUnhealthy(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "idle", "disconnected", "not_initialized", "stopped", "paused", "starved", "starving", "rejected", "stale", "hsp_paused_on_starving", "hsp_starving", "hsp_state_paused", "hsp_state_starving":
		return true
	default:
		return false
	}
}

// MotionTimingCapabilities takes the stricter value of each limit.
func (f *FanOut) MotionTimingCapabilities() MotionTimingCapabilities {
	primary := timingOf(f.primary)
	companion := timingOf(f.companion)
	return MotionTimingCapabilities{
		MinimumPointInterval:     max(primary.MinimumPointInterval, companion.MinimumPointInterval),
		MinimumBufferedLead:      max(primary.MinimumBufferedLead, companion.MinimumBufferedLead),
		MinimumMediaBufferedLead: max(primary.MinimumMediaBufferedLead, companion.MinimumMediaBufferedLead),
	}
}

// MotionSamplingCapabilities follows the primary's encoding limits.
func (f *FanOut) MotionSamplingCapabilities() MotionSamplingCapabilities {
	if provider, ok := f.primary.(MotionSamplingCapabilitiesProvider); ok {
		return provider.MotionSamplingCapabilities()
	}
	return MotionSamplingCapabilities{}
}

// PlaybackStartTime follows the primary only. The companion's own pre-roll
// must not shift the Handy's clock, so the engine runs exactly as it would
// with the Handy alone.
func (f *FanOut) PlaybackStartTime() time.Time {
	if provider, ok := f.primary.(PlaybackStartTimeProvider); ok {
		return provider.PlaybackStartTime()
	}
	return time.Time{}
}

func timingOf(owner Transport) MotionTimingCapabilities {
	if provider, ok := owner.(MotionTimingCapabilitiesProvider); ok {
		return provider.MotionTimingCapabilities()
	}
	return MotionTimingCapabilities{}
}
