package motion

import (
	"context"
	"errors"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) dispatchNextChunk(ctx context.Context, runEpoch uint64, reason string) error {
	if err := e.ensurePlaybackHealthy(ctx, runEpoch, reason); err != nil {
		return err
	}

	e.commandMu.Lock()
	streamID, points, lastSample, err := e.nextChunk(runEpoch)
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
	e.mu.Lock()
	if e.validateRunLocked(runEpoch) == nil {
		requiredTail := e.estimatedPlaybackMillisLocked(e.now()) + e.leadMillisLocked()
		needsLead = e.nextSampleMillis < requiredTail
	} else {
		e.mu.Unlock()
		return errRunInvalidated
	}
	e.mu.Unlock()
	if !needsLead {
		return nil
	}
	return e.dispatchNextChunk(ctx, runEpoch, reason)
}

func (e *Engine) setStrokeWindow(ctx context.Context, runEpoch uint64, reason string, recoverFailure bool) error {
	e.commandMu.Lock()
	settings, err := e.motionSettings(runEpoch)
	if err != nil {
		e.commandMu.Unlock()
		return err
	}
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		e.commandMu.Unlock()
		return err
	}
	strokeCommand := transport.StrokeWindowCommand{
		MinPercent:       settings.StrokeMinPercent,
		MaxPercent:       settings.StrokeMaxPercent,
		ReverseDirection: settings.ReverseDirection,
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

func (e *Engine) nextChunk(runEpoch uint64) (string, []transport.TimedPoint, *MotionSample, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return "", nil, nil, err
	}

	samples, err := e.nextMotionSamplesLocked()
	if err != nil {
		return "", nil, nil, err
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
	e.recordTransportResultWithAnnotation(reason, nil, transport.Command{
		Kind: transport.CommandKindStop,
		Stop: &stopCommand,
	}, result, stopErr, "startup_failure=true")
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
