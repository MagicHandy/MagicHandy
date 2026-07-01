package motion

import (
	"context"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) dispatchNextChunk(ctx context.Context, reason string) error {
	if err := e.ensurePlaybackHealthy(ctx, reason); err != nil {
		return err
	}
	streamID, points, lastSample := e.nextChunk()
	if len(points) == 0 {
		return nil
	}

	result, err := e.transport.AddHSP(ctx, transport.HSPAddCommand{
		StreamID: streamID,
		Points:   points,
	})
	e.recordTransportResult(reason, lastSample, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) dispatchIfLeadNeeded(ctx context.Context, reason string) error {
	if err := e.ensurePlaybackHealthy(ctx, reason); err != nil {
		return err
	}
	needsLead := false
	e.mu.Lock()
	if e.running {
		requiredTail := e.estimatedPlaybackMillisLocked(e.now()) + e.leadMillisLocked()
		needsLead = e.nextSampleMillis < requiredTail
	}
	e.mu.Unlock()
	if !needsLead {
		return nil
	}
	return e.dispatchNextChunk(ctx, reason)
}

func (e *Engine) setStrokeWindow(ctx context.Context, reason string) error {
	settings := e.motionSettings()
	result, err := e.transport.SetStrokeWindow(ctx, transport.StrokeWindowCommand{
		MinPercent: settings.StrokeMinPercent,
		MaxPercent: settings.StrokeMaxPercent,
	})
	e.recordTransportResult(reason, nil, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) play(ctx context.Context) error {
	streamID := e.currentStreamID()
	result, err := e.transport.PlayHSP(ctx, transport.HSPPlayCommand{StreamID: streamID})
	e.recordTransportResult("play", nil, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) nextChunk() (string, []transport.TimedPoint, *MotionSample) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return "", nil, nil
	}

	points := make([]transport.TimedPoint, e.chunkSize)
	var lastSample MotionSample
	for index := range points {
		streamMillis := e.nextSampleMillis + int64(index)*e.sampleInterval.Milliseconds()
		sample := e.plan.SampleAt(streamMillis)
		if e.bridgeSample != nil && streamMillis >= e.bridgeSample.TimeMillis {
			sample = *e.bridgeSample
			sample.TimeMillis = streamMillis
			e.bridgeSample = nil
		}
		points[index] = transport.TimedPoint{
			PositionPercent: e.transportPositionLocked(sample.PositionPercent),
			TimeMillis:      sample.TimeMillis,
		}
		lastSample = sample
	}
	e.nextSampleMillis += int64(e.chunkSize) * e.sampleInterval.Milliseconds()
	e.lastSample = &lastSample
	return e.streamID, points, &lastSample
}

func (e *Engine) currentStreamID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.streamID
}

func (e *Engine) motionSettings() config.MotionSettings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings
}

func (e *Engine) transportPositionLocked(position int) int {
	if e.settings.ReverseDirection {
		return 100 - position
	}
	return position
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

func (e *Engine) forceStopped(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = false
	e.cancel = nil
	e.done = nil
	if err != nil {
		e.lastError = err.Error()
	}
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
