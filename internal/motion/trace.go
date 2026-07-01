package motion

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) snapshotLocked() ActiveMotionState {
	state := ActiveMotionState{
		Running:          e.running,
		Generation:       e.generation,
		StreamID:         e.streamID,
		PlanID:           e.plan.ID,
		Target:           e.plan.Target,
		Settings:         e.settings,
		NextSampleMillis: e.nextSampleMillis,
		LastError:        redactedError(e.lastError),
	}
	if !e.startedAt.IsZero() {
		state.StartedAt = e.startedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	if e.plan.ID != "" {
		state.Phase = e.plan.PhaseAt(e.nextSampleMillis)
	}
	if e.lastSample != nil {
		sample := *e.lastSample
		state.LastSample = &sample
	}
	if e.lastResult != nil {
		result := transport.SafeCommandResult(*e.lastResult)
		state.LastResult = &result
	}
	return state
}

func (e *Engine) traceStateLocked(reason string, annotation string) {
	if e.traces == nil {
		return
	}
	e.traces.Add(diagnostics.MotionTraceRow{
		Source:     e.plan.Target.Source,
		Reason:     reason,
		Target:     traceTarget(e.plan.Target, e.settings),
		Annotation: annotation,
	})
}

func (e *Engine) recordTransportResult(
	reason string,
	sample *MotionSample,
	result transport.CommandResult,
	err error,
) {
	if e.traces == nil {
		return
	}

	diagnosticsSnapshot := e.transport.Diagnostics()
	row := diagnostics.MotionTraceRow{
		Source:          e.traceSource(),
		Reason:          reason,
		Target:          e.traceTargetSnapshot(),
		Sample:          traceSample(sample),
		TransportResult: safeResultPointer(result),
	}
	if diagnosticsSnapshot.LastCommand != nil {
		command := transport.SafeCommand(*diagnosticsSnapshot.LastCommand)
		row.TransportCommand = &command
	}
	if err != nil {
		row.Annotation = "transport_error"
	}
	e.traces.Add(row)
}

func (e *Engine) traceSource() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.plan.Target.Source == "" {
		return "motion"
	}
	return e.plan.Target.Source
}

func (e *Engine) traceTargetSnapshot() *diagnostics.MotionTraceTarget {
	e.mu.Lock()
	defer e.mu.Unlock()
	return traceTarget(e.plan.Target, e.settings)
}

func traceTarget(target MotionTarget, settings config.MotionSettings) *diagnostics.MotionTraceTarget {
	return &diagnostics.MotionTraceTarget{
		Label:             target.Label,
		SpeedPercent:      target.SpeedPercent,
		StrokeMinPercent:  settings.StrokeMinPercent,
		StrokeMaxPercent:  settings.StrokeMaxPercent,
		ReverseDirection:  settings.ReverseDirection,
		PatternIdentifier: string(target.PatternID),
	}
}

func traceSample(sample *MotionSample) *diagnostics.MotionTraceSample {
	if sample == nil {
		return nil
	}
	return &diagnostics.MotionTraceSample{
		PositionPercent: sample.PositionPercent,
		TimeMillis:      sample.TimeMillis,
	}
}

func safeResultPointer(result transport.CommandResult) *transport.CommandResult {
	if result.Transport == "" && result.Kind == "" {
		return nil
	}
	safeResult := transport.SafeCommandResult(result)
	return &safeResult
}

func (e *Engine) planIDLocked() string {
	return fmt.Sprintf("%s-%06d", e.streamID, e.generation)
}

func phaseAnnotation(preserved bool) string {
	if preserved {
		return "phase_preserved=true"
	}
	return "phase_preserved=false"
}

func redactedError(value string) string {
	if value == "" {
		return ""
	}
	return "redacted"
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
