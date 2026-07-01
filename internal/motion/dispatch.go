package motion

import (
	"context"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) dispatchNextChunk(ctx context.Context, reason string) error {
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
