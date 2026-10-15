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

// AlignedPlayer starts a companion on another participant's clock: stream
// millisecond 0 is start, so it never renders against its own provisional
// anchor before a linked session aligns it.
type AlignedPlayer interface {
	PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error)
}

// LinkedParticipant is one device in a linked playback session. ID is the
// stable device identifier (handy, coyote, vibrator); Label names it in
// errors and warnings.
type LinkedParticipant struct {
	ID        string
	Label     string
	Transport Transport
}

// FanOut is one linked playback session. The first participant is the run
// clock: companions start on its playback origin, and sampling and startup
// state follow it. A fault latches until a new session is constructed, so
// reconnecting cannot silently rejoin an existing run.
type FanOut struct {
	participants     []LinkedParticipant
	mu               sync.Mutex
	playing          bool
	fault            error
	continueHealthy  bool
	retired          []bool
	warnings         string
	faultStopped     bool
	faultParticipant string
}

type fanOutWithStartup struct {
	*FanOut
	startup MotionStartupStateProvider
}

// NewFanOut creates a Handy + Coyote linked session with stop-all failure
// handling.
func NewFanOut(primary, companion Transport) Transport {
	return NewFanOutWithPolicy(primary, companion, false)
}

// NewFanOutWithPolicy creates a Handy + Coyote linked session that optionally
// retires a failed participant during playback.
func NewFanOutWithPolicy(primary, companion Transport, continueHealthy bool) Transport {
	return NewLinkedSession([]LinkedParticipant{
		{ID: "handy", Label: "Handy", Transport: primary},
		{ID: "coyote", Label: "Coyote", Transport: companion},
	}, continueHealthy)
}

// NewLinkedSession links participants in run-clock order. With
// continueHealthy, an established run retires a failed participant while any
// other stays healthy; startup still requires every participant, and a
// retired participant cannot hot-join.
func NewLinkedSession(participants []LinkedParticipant, continueHealthy bool) Transport {
	fanOut := &FanOut{
		participants:    append([]LinkedParticipant(nil), participants...),
		continueHealthy: continueHealthy,
		retired:         make([]bool, len(participants)),
	}
	if len(participants) > 0 {
		if startup, ok := participants[0].Transport.(MotionStartupStateProvider); ok {
			return &fanOutWithStartup{FanOut: fanOut, startup: startup}
		}
	}
	return fanOut
}

func (f *fanOutWithStartup) ReadMotionStartupState(ctx context.Context) (MotionStartupState, MotionStartupStateResults, error) {
	state, results, err := f.startup.ReadMotionStartupState(ctx)
	if err != nil {
		f.latch(f.participants[0].Label, err)
	}
	return state, results, err
}

// Stop always reaches every participant, even if one fails or was retired.
// Fault stops zero electrical outputs; an ordinary Pause retains strength.
func (f *FanOut) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	f.mu.Lock()
	command.Fault = command.Fault || f.fault != nil && !f.faultStopped
	f.playing = false
	f.mu.Unlock()
	results := make([]CommandResult, len(f.participants))
	errs := make([]error, len(f.participants))
	var wait sync.WaitGroup
	for index := 1; index < len(f.participants); index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = f.participants[index].Transport.Stop(ctx, command)
		}(index)
	}
	if len(f.participants) > 0 {
		results[0], errs[0] = f.participants[0].Transport.Stop(ctx, command)
	}
	wait.Wait()
	labeled := make([]error, len(errs))
	for index, err := range errs {
		labeled[index] = labelError(f.participants[index].Label, err)
	}
	err := errors.Join(labeled...)
	var result CommandResult
	if len(results) > 0 {
		result = results[0]
	}
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
	return f.dispatch(ctx, func(_ int, t Transport) (CommandResult, error) { return t.SetStrokeWindow(ctx, command) })
}

// AppendPoints sends the shared engine's points to each active participant.
func (f *FanOut) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	return f.dispatch(ctx, func(_ int, t Transport) (CommandResult, error) { return t.AppendPoints(ctx, command) })
}

// Play starts the clock participant first, then starts every companion on
// its playback origin before any companion output is enabled.
func (f *FanOut) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	var clockStart time.Time
	clock := -1
	result, err := f.dispatch(ctx, func(index int, t Transport) (CommandResult, error) {
		if clock < 0 {
			next, err := t.Play(ctx, command)
			if err != nil {
				// A retired clock hands the clock to the next participant.
				return next, err
			}
			clock = index
			if provider, ok := t.(PlaybackStartTimeProvider); ok {
				clockStart = provider.PlaybackStartTime()
			}
			return next, nil
		}
		return playCompanion(ctx, t, command, clockStart)
	})
	if err != nil {
		return result, err
	}
	f.mu.Lock()
	f.playing = true
	f.mu.Unlock()
	return result, nil
}

func playCompanion(ctx context.Context, companion Transport, command PlayCommand, start time.Time) (CommandResult, error) {
	if aligned, ok := companion.(AlignedPlayer); ok && !start.IsZero() {
		return aligned.PlayAligned(ctx, command, start)
	}
	result, err := companion.Play(ctx, command)
	if err == nil && !start.IsZero() {
		if aligner, ok := companion.(playbackAligner); ok {
			aligner.AlignPlaybackStart(start)
		}
	}
	return result, err
}

func (f *FanOut) dispatch(ctx context.Context, send func(int, Transport) (CommandResult, error)) (CommandResult, error) {
	f.mu.Lock()
	fault, retired := f.fault, append([]bool(nil), f.retired...)
	f.mu.Unlock()
	if fault != nil {
		return CommandResult{}, fault
	}
	var result CommandResult
	for index, participant := range f.participants {
		if retired[index] {
			continue
		}
		next, err := send(index, participant.Transport)
		if err != nil {
			if f.retire(ctx, index, err) {
				continue
			}
			return next, f.failedCommand(ctx, participant.Label, err)
		}
		if !result.OK {
			result = next
		}
	}
	return result, nil
}

// retire only applies to an established run with the explicit continue
// policy, and only while another participant stays healthy.
func (f *FanOut) retire(ctx context.Context, index int, cause error) bool {
	f.mu.Lock()
	if !f.playing || !f.continueHealthy || f.healthyOthersLocked(index) == 0 {
		f.mu.Unlock()
		return false
	}
	already := f.retired[index]
	f.retired[index] = true
	label := f.participants[index].Label
	f.warnings = label + " failed; continuing on the other devices. Reconnect and start again to rejoin."
	f.mu.Unlock()
	if already {
		return true
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := f.participants[index].Transport.Stop(stopCtx, StopCommand{Reason: "linked_participant_failed", Fault: true})
	if err != nil {
		f.mu.Lock()
		f.warnings += " Stop could not be confirmed."
		f.mu.Unlock()
	}
	_ = cause // Never expose raw transport errors in diagnostic summaries.
	return true
}

func (f *FanOut) healthyOthersLocked(index int) int {
	count := 0
	for other, retired := range f.retired {
		if other != index && !retired {
			count++
		}
	}
	return count
}

// Stop every participant as soon as a command fails; the engine then
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
	states := make([]TransportDiagnostics, len(f.participants))
	stopping := false
	for index, participant := range f.participants {
		states[index] = participant.Transport.Diagnostics()
		stopping = stopping || index > 0 && states[index].PlaybackState == "stopping"
	}
	f.mu.Lock()
	playing, fault, retired := f.playing, f.fault, append([]bool(nil), f.retired...)
	f.mu.Unlock()
	if fault == nil && playing && !stopping {
		for index, state := range states {
			if retired[index] {
				continue
			}
			if !state.Connected || linkedPlaybackUnhealthy(state.PlaybackState) {
				cause := fmt.Errorf("playback lost (%s); linked session stopped", state.PlaybackState)
				if !f.retire(context.Background(), index, cause) {
					f.latch(f.participants[index].Label, cause)
				}
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(states) == 0 {
		return TransportDiagnostics{}
	}
	// Report the clock participant, or the first one still playing.
	reported := -1
	connected := true
	for index, state := range states {
		if f.retired[index] {
			continue
		}
		if reported < 0 {
			reported = index
		}
		connected = connected && state.Connected
	}
	result := states[0]
	if reported >= 0 {
		result = states[reported]
	}
	result.Connected = connected
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

// MotionTimingCapabilities takes the strictest value of each limit.
func (f *FanOut) MotionTimingCapabilities() MotionTimingCapabilities {
	var result MotionTimingCapabilities
	for _, participant := range f.participants {
		timing := timingOf(participant.Transport)
		result.MinimumPointInterval = max(result.MinimumPointInterval, timing.MinimumPointInterval)
		result.MinimumBufferedLead = max(result.MinimumBufferedLead, timing.MinimumBufferedLead)
		result.MinimumMediaBufferedLead = max(result.MinimumMediaBufferedLead, timing.MinimumMediaBufferedLead)
	}
	return result
}

// MotionSamplingCapabilities follows the clock participant's encoding limits.
func (f *FanOut) MotionSamplingCapabilities() MotionSamplingCapabilities {
	if len(f.participants) > 0 {
		if provider, ok := f.participants[0].Transport.(MotionSamplingCapabilitiesProvider); ok {
			return provider.MotionSamplingCapabilities()
		}
	}
	return MotionSamplingCapabilities{}
}

// PlaybackStartTime follows the clock participant only. A companion's own
// pre-roll must not shift the clock, so the engine runs exactly as it would
// with the clock participant alone.
func (f *FanOut) PlaybackStartTime() time.Time {
	if len(f.participants) > 0 {
		if provider, ok := f.participants[0].Transport.(PlaybackStartTimeProvider); ok {
			return provider.PlaybackStartTime()
		}
	}
	return time.Time{}
}

func timingOf(owner Transport) MotionTimingCapabilities {
	if provider, ok := owner.(MotionTimingCapabilitiesProvider); ok {
		return provider.MotionTimingCapabilities()
	}
	return MotionTimingCapabilities{}
}
