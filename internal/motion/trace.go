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
		Paused:           e.paused,
		RunningMillis:    e.runningMillisLocked(),
		Generation:       e.generation,
		StreamID:         e.streamID,
		PlanID:           e.plan.ID,
		Target:           cloneMotionTarget(e.plan.Target),
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

// runningMillisLocked is the stopwatch value: accumulated run time across
// pauses, plus the live segment while running. Stop resets it to zero.
func (e *Engine) runningMillisLocked() int64 {
	if e.running {
		return e.runMillisAccum + e.estimatedPlaybackMillisLocked(e.now())
	}
	if e.paused {
		return e.runMillisAccum
	}
	return 0
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

func (e *Engine) traceRetargetLocked(
	reason string,
	previous MotionPlan,
	previousSettings config.MotionSettings,
	next MotionPlan,
	nextSettings config.MotionSettings,
	current MotionSample,
	handoffMillis int64,
	leadMillis int64,
	recentLatencyMillis int64,
	bridgeInserted bool,
	recovery string,
) {
	if e.traces == nil {
		return
	}
	e.traces.Add(diagnostics.MotionTraceRow{
		Source:     next.Target.Source,
		Reason:     reason,
		Target:     traceTarget(next.Target, nextSettings),
		Sample:     traceSample(&current),
		Annotation: retargetAnnotation(next.PhasePreserved, bridgeInserted),
		Retarget: &diagnostics.MotionTraceRetarget{
			PreviousPlanID:                  previous.ID,
			NextPlanID:                      next.ID,
			PreviousPatternIdentifier:       string(previous.PatternID),
			NextPatternIdentifier:           string(next.PatternID),
			PreviousTarget:                  traceTarget(previous.Target, previousSettings),
			NextTarget:                      traceTarget(next.Target, nextSettings),
			EstimatedCurrentPositionPercent: current.PositionPercent,
			EstimatedCurrentStreamMillis:    current.TimeMillis,
			SelectedHandoffMillis:           handoffMillis,
			SelectedLeadMillis:              leadMillis,
			RecentCommandLatencyMillis:      recentLatencyMillis,
			PhasePreserved:                  next.PhasePreserved,
			BridgePointsInserted:            bridgeInserted,
			Recovery:                        recovery,
		},
	})
}

func (e *Engine) recordTransportResult(
	reason string,
	sample *MotionSample,
	result transport.CommandResult,
	err error,
) {
	e.recordTransportResultWithAnnotation(reason, sample, result, err, "")
}

func (e *Engine) recordTransportResultWithAnnotation(
	reason string,
	sample *MotionSample,
	result transport.CommandResult,
	err error,
	annotation string,
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
		Annotation:      annotation,
	}
	if diagnosticsSnapshot.LastCommand != nil {
		command := transport.SafeCommand(*diagnosticsSnapshot.LastCommand)
		row.TransportCommand = &command
	}
	if err != nil {
		if row.Annotation == "" {
			row.Annotation = "transport_error"
		}
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
	trace := &diagnostics.MotionTraceTarget{
		Label:             target.Label,
		SpeedPercent:      target.SpeedPercent,
		StrokeMinPercent:  settings.StrokeMinPercent,
		StrokeMaxPercent:  settings.StrokeMaxPercent,
		ReverseDirection:  settings.ReverseDirection,
		PatternIdentifier: string(target.PatternID),
	}
	if target.AreaFocus != nil {
		trace.AreaMinPercent = target.AreaFocus.MinPercent
		trace.AreaMaxPercent = target.AreaFocus.MaxPercent
	}
	if target.SoftAnchor != nil {
		trace.SoftAnchorPositionPercent = target.SoftAnchor.PositionPercent
		trace.SoftAnchorWeightPercent = target.SoftAnchor.WeightPercent
	}
	return trace
}

func cloneMotionTarget(target MotionTarget) MotionTarget {
	if target.AreaFocus != nil {
		area := *target.AreaFocus
		target.AreaFocus = &area
	}
	if target.SoftAnchor != nil {
		anchor := *target.SoftAnchor
		target.SoftAnchor = &anchor
	}
	return target
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

func retargetAnnotation(phasePreserved bool, bridgeInserted bool) string {
	annotation := phaseAnnotation(phasePreserved)
	if bridgeInserted {
		annotation += ";bridge_points=true"
	}
	return annotation
}

func redactedError(value string) string {
	if value == "" {
		return ""
	}
	return "redacted"
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
