package motion

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (e *Engine) snapshotLocked() ActiveMotionState {
	now := e.now()
	playbackMillis := int64(0)
	if e.running {
		playbackMillis = e.estimatedPlaybackMillisLocked(now)
	}
	runningMillis := int64(0)
	if e.running {
		runningMillis = e.runMillisAccum + playbackMillis
	} else if e.paused {
		runningMillis = e.runMillisAccum
	}
	state := ActiveMotionState{
		Running:                    e.running,
		Starting:                   e.starting,
		Completing:                 e.completing,
		Paused:                     e.paused,
		RunningMillis:              runningMillis,
		Generation:                 e.generation,
		StreamID:                   e.streamID,
		PlanID:                     e.plan.ID,
		Target:                     cloneMotionTarget(e.plan.Target),
		Settings:                   e.settings,
		NextSampleMillis:           e.nextSampleMillis,
		RecentCommandLatencyMillis: e.recentCommandLatencyMillisLocked(),
		LastError:                  redactedError(e.lastError),
		Perceptual:                 e.plan.Perceptual,
	}
	if e.plan.Target.Dynamic != nil && e.plan.ID != "" {
		pace := e.plan.Perceptual.Pace
		pace.Limiters = append([]string(nil), pace.Limiters...)
		state.Pace = &pace
	}
	if !e.startedAt.IsZero() {
		state.StartedAt = e.startedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	if e.plan.ID != "" {
		state.Phase = e.frozenPhase
		if e.running {
			state.Phase = e.plan.PhaseAt(playbackMillis)
		}
	}
	if e.running && e.plan.ID != "" {
		current := e.currentSample
		if !e.starting {
			live := sampleMotionPath(e.plan, e.transition, playbackMillis)
			current = &live
		}
		if current != nil {
			sample := *current
			state.CurrentSample = &sample
		}
	} else if (e.paused || e.completing) && e.currentSample != nil {
		sample := *e.currentSample
		state.CurrentSample = &sample
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
		Target:     traceTargetWithPace(e.plan.Target, e.settings, e.plan.Perceptual.Pace),
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
		Target:     traceTargetWithPace(next.Target, nextSettings, next.Perceptual.Pace),
		Sample:     traceSample(&current),
		Annotation: retargetAnnotation(next.PhasePreserved, bridgeInserted),
		Retarget: &diagnostics.MotionTraceRetarget{
			PreviousPlanID:                  previous.ID,
			NextPlanID:                      next.ID,
			PreviousPatternIdentifier:       string(previous.PatternID),
			NextPatternIdentifier:           string(next.PatternID),
			PreviousProgramIdentifier:       previous.ProgramID,
			NextProgramIdentifier:           next.ProgramID,
			PreviousTarget:                  traceTargetWithPace(previous.Target, previousSettings, previous.Perceptual.Pace),
			NextTarget:                      traceTargetWithPace(next.Target, nextSettings, next.Perceptual.Pace),
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
	command transport.Command,
	result transport.CommandResult,
	err error,
) {
	e.recordTransportResultWithAnnotation(reason, sample, command, result, err, "")
}

func (e *Engine) recordTransportResultWithAnnotation(
	reason string,
	sample *MotionSample,
	command transport.Command,
	result transport.CommandResult,
	err error,
	annotation string,
) {
	if e.traces == nil {
		return
	}

	row := diagnostics.MotionTraceRow{
		Source:          e.traceSource(),
		Reason:          reason,
		Target:          e.traceTargetSnapshot(),
		Sample:          traceSample(sample),
		TransportResult: safeResultPointer(result),
		Annotation:      annotation,
	}
	if command.Kind != "" {
		command.ID = result.CommandID
		safeCommand := transport.SafeCommand(command)
		row.TransportCommand = &safeCommand
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
	return traceTargetWithPace(e.plan.Target, e.settings, e.plan.Perceptual.Pace)
}

func traceTarget(target MotionTarget, settings config.MotionSettings) *diagnostics.MotionTraceTarget {
	return traceTargetWithPace(target, settings, PaceSummary{})
}

func traceTargetWithPace(target MotionTarget, settings config.MotionSettings, pace PaceSummary) *diagnostics.MotionTraceTarget {
	trace := &diagnostics.MotionTraceTarget{
		Label:                  target.Label,
		SpeedPercent:           target.SpeedPercent,
		StrokeMinPercent:       settings.StrokeMinPercent,
		StrokeMaxPercent:       settings.StrokeMaxPercent,
		ReverseDirection:       settings.ReverseDirection,
		PatternIdentifier:      string(target.PatternID),
		ProgramIdentifier:      target.ProgramID,
		MediaIdentifier:        target.MediaID,
		MediaSpeedLimitEnabled: target.MediaSpeedLimitEnabled,
	}
	if target.AreaFocus != nil {
		trace.AreaMinPercent = target.AreaFocus.MinPercent
		trace.AreaMaxPercent = target.AreaFocus.MaxPercent
	}
	if target.SoftAnchor != nil {
		trace.SoftAnchorPositionPercent = target.SoftAnchor.PositionPercent
		trace.SoftAnchorWeightPercent = target.SoftAnchor.WeightPercent
	}
	if target.Dynamic != nil {
		dynamic := NormalizeDynamicDefinition(*target.Dynamic)
		trace.MotionKind = "dynamic"
		trace.DynamicCenterPercent = dynamic.CenterPercent
		trace.DynamicSpanPercent = dynamic.SpanPercent
		trace.DynamicSpanMinPercent = dynamic.SpanMinPercent
		trace.DynamicSpanProfile = dynamic.SpanProfile
		trace.DynamicPhraseSeed = dynamic.PhraseSeed
		trace.DynamicVariationPercent = dynamic.VariationPercent
		trace.DynamicSegmentSeconds = dynamic.SegmentSeconds
		trace.DynamicSectionCount = len(dynamic.Sections)
		for _, anchor := range dynamic.Anchors {
			trace.DynamicAnchors = append(trace.DynamicAnchors, anchor.Name)
		}
		if pace.RequestedPercent > 0 {
			trace.EffectiveSpeedPercent = pace.EffectivePercent
			trace.PaceLimited = pace.Limited
			trace.PaceLimiters = append([]string(nil), pace.Limiters...)
			trace.CommandedMeanTravel = pace.CommandedMeanTravelPerSecond
			trace.CommandedPeakVelocity = pace.CommandedPeakVelocityPerSecond
			trace.DevicePeakVelocity = pace.DevicePeakVelocityPerSecond
		}
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
	if target.Pattern != nil {
		definition := clonePatternDefinition(*target.Pattern)
		target.Pattern = &definition
	}
	if target.Dynamic != nil {
		dynamic := *target.Dynamic
		dynamic.Anchors = append([]DynamicAnchor(nil), target.Dynamic.Anchors...)
		dynamic.Sections = cloneDynamicSections(target.Dynamic.Sections)
		target.Dynamic = &dynamic
	}
	if target.Program != nil {
		definition := *target.Program
		definition.Points = append([]CurvePoint(nil), target.Program.Points...)
		target.Program = &definition
	}
	// Media curves can contain 100k points and are never part of the public
	// snapshot. Keep the stable ID/label while avoiding a multi-megabyte copy on
	// every motion-state poll.
	target.Media = nil
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
