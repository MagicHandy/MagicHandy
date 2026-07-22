package motion

import (
	"context"
	"errors"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) dispatchNextChunk(ctx context.Context, runEpoch uint64, reason string) error {
	return e.dispatchThrough(ctx, runEpoch, reason, 0)
}

func (e *Engine) dispatchThrough(
	ctx context.Context,
	runEpoch uint64,
	reason string,
	targetTailMillis int64,
) error {
	if err := e.ensurePlaybackHealthy(ctx, runEpoch, reason); err != nil {
		return err
	}

	e.commandMu.Lock()
	streamID, points, lastSample, err := e.nextChunkThrough(runEpoch, targetTailMillis)
	if err != nil {
		e.commandMu.Unlock()
		return err
	}
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		e.commandMu.Unlock()
		return err
	}

	appendCommand := transport.AppendPointsCommand{
		StreamID: streamID,
		Points:   points,
	}
	result, err := e.transport.AppendPoints(commandCtx, appendCommand)
	cleanup()
	e.recordTransportResult(reason, lastSample, transport.Command{
		Kind:      transport.CommandKindPointsAdd,
		PointsAdd: &appendCommand,
	}, result, err)
	e.rememberResult(result, err)
	if err == nil || ctx.Err() != nil || errors.Is(err, errRunInvalidated) || e.validateRun(runEpoch) != nil {
		e.commandMu.Unlock()
		return err
	}

	message := fmt.Sprintf("motion recovery stopped active stream after point dispatch failed during %s", reason)
	recovery, prepareErr := e.prepareRecovery(runEpoch, message)
	e.commandMu.Unlock()
	if prepareErr != nil {
		return prepareErr
	}
	return e.finishRecovery(ctx, "recovery_"+reason, message, recovery)
}

func (e *Engine) dispatchIfLeadNeeded(ctx context.Context, runEpoch uint64, reason string) error {
	if err := e.ensurePlaybackHealthy(ctx, runEpoch, reason); err != nil {
		return err
	}
	needsLead := false
	var targetTail int64
	e.mu.Lock()
	if e.validateRunLocked(runEpoch) == nil {
		requiredTail := e.estimatedPlaybackMillisLocked(e.now()) + e.leadMillisLocked()
		needsLead = e.bufferedTailMillisLocked() < requiredTail
		if needsLead {
			targetTail = e.refillTailMillisLocked(requiredTail)
		}
	} else {
		e.mu.Unlock()
		return errRunInvalidated
	}
	e.mu.Unlock()
	if !needsLead {
		return nil
	}
	return e.dispatchThrough(ctx, runEpoch, reason, targetTail)
}

func (e *Engine) refillTailMillisLocked(requiredTail int64) int64 {
	if e.plan.Target.Media == nil || e.minimumMediaBufferedLeadMillis <= 0 {
		return 0
	}
	// Refill fixed media in fewer, deeper batches. The point cap still bounds
	// each append, while interactive targets keep their shorter retarget horizon.
	return requiredTail + min(e.minimumMediaBufferedLeadMillis/2, int64(4000))
}

// prebufferBeforePlay gives high-latency buffered owners enough accepted
// coverage to survive their first append after playback begins. Owners without
// a declared buffered-lead requirement retain the ordinary one-chunk startup.
func (e *Engine) prebufferBeforePlay(ctx context.Context, runEpoch uint64, reason string) error {
	for range 64 {
		e.mu.Lock()
		if err := e.validateRunLocked(runEpoch); err != nil {
			e.mu.Unlock()
			return err
		}
		requiredLead := e.selectedMinimumBufferedLeadMillisLocked()
		if requiredLead > 0 {
			requiredLead = e.leadMillisLocked()
		}
		dispatchTail := int64(0)
		if e.plan.Target.Media != nil {
			dispatchTail = requiredLead
		}
		ready := requiredLead <= 0 || e.bufferedTailMillisLocked() >= requiredLead
		e.mu.Unlock()
		if ready {
			return nil
		}
		if err := e.dispatchThrough(ctx, runEpoch, reason, dispatchTail); err != nil {
			return err
		}
	}
	return errors.New("motion startup could not build the transport's minimum lead buffer")
}

func (e *Engine) setStrokeWindow(ctx context.Context, runEpoch uint64, reason string, recoverFailure bool) error {
	settings, err := e.motionSettings(runEpoch)
	if err != nil {
		return err
	}
	return e.setStrokeWindowCommand(ctx, runEpoch, reason, transport.StrokeWindowCommand{
		MinPercent:       settings.StrokeMinPercent,
		MaxPercent:       settings.StrokeMaxPercent,
		ReverseDirection: settings.ReverseDirection,
	}, recoverFailure)
}

func (e *Engine) setStrokeWindowCommand(
	ctx context.Context,
	runEpoch uint64,
	reason string,
	strokeCommand transport.StrokeWindowCommand,
	recoverFailure bool,
) error {
	e.commandMu.Lock()
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		e.commandMu.Unlock()
		return err
	}
	result, err := e.transport.SetStrokeWindow(commandCtx, strokeCommand)
	cleanup()
	e.recordTransportResult(reason, nil, transport.Command{
		Kind:         transport.CommandKindStrokeWindow,
		StrokeWindow: &strokeCommand,
	}, result, err)
	e.rememberResult(result, err)
	if err == nil || !recoverFailure || ctx.Err() != nil || e.validateRun(runEpoch) != nil {
		e.commandMu.Unlock()
		return err
	}

	message := fmt.Sprintf("motion recovery stopped active stream after stroke-window update failed during %s", reason)
	recovery, prepareErr := e.prepareRecovery(runEpoch, message)
	e.commandMu.Unlock()
	if prepareErr != nil {
		return prepareErr
	}
	return e.finishRecovery(ctx, "recovery_"+reason, message, recovery)
}

func (e *Engine) play(ctx context.Context, runEpoch uint64) error {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	streamID, err := e.currentStreamID(runEpoch)
	if err != nil {
		return err
	}
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		return err
	}
	defer cleanup()
	playCommand := transport.PlayCommand{StreamID: streamID}
	result, err := e.transport.Play(commandCtx, playCommand)
	e.recordTransportResult("play", nil, transport.Command{
		Kind:       transport.CommandKindPointsPlay,
		PointsPlay: &playCommand,
	}, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) nextChunkThrough(
	runEpoch uint64,
	targetTailMillis int64,
) (string, []transport.TimedPoint, *MotionSample, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return "", nil, nil, err
	}

	linearMedia := e.plan.Target.Media != nil && e.transition == nil && e.preservePlanKnots
	var samples []MotionSample
	var err error
	if linearMedia {
		samples, err = e.nextLinearMediaSamplesLocked(targetTailMillis)
	} else {
		samples, err = e.nextMotionSamplesLocked()
	}
	if err != nil {
		return "", nil, nil, err
	}
	for !linearMedia && samples[len(samples)-1].TimeMillis < targetTailMillis {
		previousNextSampleMillis := e.nextSampleMillis
		previousLastSample := cloneMotionSample(e.lastSample)
		additional, additionalErr := e.nextMotionSamplesLocked()
		if additionalErr != nil {
			return "", nil, nil, additionalErr
		}
		if len(samples)+len(additional) > e.maximumChunkPoints {
			e.nextSampleMillis = previousNextSampleMillis
			e.lastSample = previousLastSample
			break
		}
		samples = append(samples, additional...)
	}
	points := make([]transport.TimedPoint, len(samples))
	for index, sample := range samples {
		// Emit the semantic 0..100 travel position. Reverse direction is a
		// transport-boundary mapping (docs/hsp-v4-invariants.md, Invariant 3):
		// the Cloud REST and Browser Bluetooth transports invert x from the
		// same setting, so inverting here too would double-invert to a no-op.
		points[index] = transport.TimedPoint{
			PositionPercent: sample.PositionPercent,
			TimeMillis:      sample.TimeMillis,
		}
	}
	lastSample := samples[len(samples)-1]
	return e.streamID, points, &lastSample, nil
}

func cloneMotionSample(sample *MotionSample) *MotionSample {
	if sample == nil {
		return nil
	}
	cloned := *sample
	return &cloned
}

func (e *Engine) currentStreamID(runEpoch uint64) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return "", err
	}
	return e.streamID, nil
}

func (e *Engine) motionSettings(runEpoch uint64) (config.MotionSettings, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return config.MotionSettings{}, err
	}
	return e.settings, nil
}

func (e *Engine) rememberResult(result transport.CommandResult, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result = transport.SafeCommandResult(result)
	e.lastResult = &result
	if result.LatencyMillis > 0 {
		e.latencyMillis = append(e.latencyMillis, result.LatencyMillis)
		if len(e.latencyMillis) > latencySampleLimit {
			e.latencyMillis = e.latencyMillis[len(e.latencyMillis)-latencySampleLimit:]
		}
	}
	if err != nil {
		e.lastError = err.Error()
		return
	}
	e.lastError = ""
}

func (e *Engine) rememberError(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastError = err.Error()
}

// forceStopped abandons a start/resume that failed during transport setup. It
// releases the loop context installed by begin/beginResume so a startup
// failure never leaks a live cancel func, and a concurrent Stop that already
// cleared e.cancel is a no-op here.
func (e *Engine) forceStopped(runEpoch uint64, err error) bool {
	e.mu.Lock()
	if e.runEpoch != runEpoch || !e.running {
		e.mu.Unlock()
		return false
	}
	cancel := e.cancel
	e.stopBarriers++
	e.frozenPhase = e.plan.PhaseAt(e.estimatedPlaybackMillisLocked(e.now()))
	e.running = false
	e.starting = false
	e.completing = false
	e.cancel = nil
	e.runCtx = nil
	e.done = nil
	if err != nil {
		e.lastError = err.Error()
	}
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return true
}

func (e *Engine) abortStartup(ctx context.Context, runEpoch uint64, reason string, cause error) {
	if cause == nil || errors.Is(cause, errRunInvalidated) || !e.forceStopped(runEpoch, cause) {
		return
	}

	e.commandMu.Lock()
	stopCtx, stopCancel := detachedStopContext(ctx)
	stopCommand := transport.StopCommand{Reason: reason}
	result, stopErr := e.transport.Stop(stopCtx, stopCommand)
	stopCancel()
	annotation := "startup_failure=true"
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		annotation = "startup_cancelled=true"
	}
	e.recordTransportResultWithAnnotation(reason, nil, transport.Command{
		Kind: transport.CommandKindStop,
		Stop: &stopCommand,
	}, result, stopErr, annotation)
	e.finishStopped(result, stopErr)
	e.mu.Lock()
	if stopErr != nil {
		e.lastError = fmt.Sprintf("%v; safety Stop failed: %v", cause, stopErr)
	} else {
		e.lastError = cause.Error()
	}
	e.endStopBarrierLocked()
	e.mu.Unlock()
	e.commandMu.Unlock()
}

func (e *Engine) activeRunEpoch() (uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return 0, errors.New("motion is not running")
	}
	if e.starting {
		return 0, errors.New("motion is still starting")
	}
	return e.runEpoch, nil
}

func (e *Engine) runEpochIfActive() (uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runEpoch, e.running && !e.starting
}

func (e *Engine) validateRun(runEpoch uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.validateRunLocked(runEpoch)
}

func (e *Engine) validateRunLocked(runEpoch uint64) error {
	if !e.running || e.runEpoch != runEpoch {
		return errRunInvalidated
	}
	return nil
}

func (e *Engine) contextForRun(ctx context.Context, runEpoch uint64) (context.Context, func(), error) {
	e.mu.Lock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		e.mu.Unlock()
		return nil, nil, err
	}
	runCtx := e.runCtx
	e.mu.Unlock()
	if runCtx == nil {
		return nil, nil, errRunInvalidated
	}
	if ctx == nil {
		ctx = context.Background()
	}
	commandCtx, cancel := context.WithCancel(ctx)
	stopRunCancel := context.AfterFunc(runCtx, cancel)
	cleanup := func() {
		stopRunCancel()
		cancel()
	}
	return commandCtx, cleanup, nil
}

func (e *Engine) finishStopped(result transport.CommandResult, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	safeResult := transport.SafeCommandResult(result)
	e.lastResult = &safeResult
	if err != nil {
		e.lastError = err.Error()
		return
	}
	e.lastError = ""
}
