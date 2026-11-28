package motion

import (
	"errors"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// ErrEdgeUnavailable identifies output that a moving or still Edge cannot control.
var ErrEdgeUnavailable = errors.New("start live position or motor-tempo playback to use Edge")

// AllowsMovingHold reports whether Edge can retarget the active output in place.
func (s ActiveMotionState) AllowsMovingHold() bool {
	return s.Running && !s.Starting && !s.Completing && !s.Paused && !s.Target.VibrationOnly &&
		s.Target.MediaID == "" && s.Target.ProgramID == ""
}

func (e *Engine) addEdgeSnapshotLocked(state *ActiveMotionState, playback int64) {
	state.EdgeAllowed = state.AllowsMovingHold()
	state.StillHold = e.paused && e.stillHold
	if state.EdgeAllowed && state.Target.Source == "edge" {
		state.EdgePhase = e.holdingPhaseLocked(playback)
	}
}

// A reserved cursor is not a receipt. A late reply for the previous path
// cannot move the displayed phase into an unacknowledged hold.
func (e *Engine) acknowledgeEdgeFrame(epoch uint64, receipt transport.CommandResult, tempo *transport.TempoFrame, tail *MotionSample) {
	if !receipt.OK || receipt.Kind != transport.CommandKindPointsAdd {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.edgeFrameAccepted || !e.running || e.runEpoch != epoch || e.plan.Target.Source != "edge" {
		return
	}
	if e.plan.Target.TempoOnly {
		e.edgeFrameAccepted = tempo != nil && e.tempo != nil && tempo.Revision == e.tempo.revision
	} else {
		e.edgeFrameAccepted = tail != nil && tail.TimeMillis >= e.edgeHandoffMillis
	}
}

// The playback cursor is a commanded estimate. Phase describes the current
// stream, including time still owned by previously queued motion.
func (e *Engine) holdingPhaseLocked(playback int64) string {
	if !e.edgeFrameAccepted {
		return "queued"
	}
	if e.plan.Target.TempoOnly && e.tempo != nil {
		if playback < e.tempo.fromMillis+int64(tempoBlendMillis) {
			return "slowing"
		}
	} else if join := e.transition; join != nil {
		start := join.startMillis
		if join.stroke != nil {
			start = join.strokeStartMillis
		}
		if playback < start {
			return "queued"
		}
		if playback < join.endMillis {
			return "slowing"
		}
	}
	return "holding"
}

// HoldingTarget is deliberately cheap and independent of inference. Keep the
// familiar working region and reduce pace inside saved limits. Use the normal
// rounded reversals: extra softness flattened turns into near-rests and then
// persisted as an invisible high-speed restriction in later Layered edits.
func HoldingTarget(state ActiveMotionState, settings config.MotionSettings) (MotionTarget, error) {
	if state.Target.VibrationOnly {
		return MotionTarget{}, ErrEdgeUnavailable
	}
	if state.Target.TempoOnly {
		if state.Tempo == nil || state.Tempo.LevelPercent < 1 {
			return MotionTarget{}, errors.New("start the motor before using Edge")
		}
		level := max(1, min(20, int(math.Round(state.Tempo.LevelPercent*0.4))))
		return MotionTarget{Source: "edge", Label: "Gentle hold", TempoOnly: true,
			Tempo: &TempoSpec{Mode: TempoModeSteady, LevelPercent: level}}, nil
	}
	lo, hi := 0, 100
	if state.Perceptual.PositionMaxPercent > state.Perceptual.PositionMinPercent || state.Perceptual.Pace.RequestedPercent > 0 {
		lo = max(0, int(math.Floor(state.Perceptual.PositionMinPercent)))
		hi = min(100, int(math.Ceil(state.Perceptual.PositionMaxPercent)))
	} else if state.Target.Flow != nil {
		lo, hi = state.Target.Flow.MinPercent, state.Target.Flow.MaxPercent
	}
	if hi-lo < 10 {
		return MotionTarget{}, errors.New("the current motion has no usable holding range")
	}
	// Pace controls travel per second. Halving the sweep at the saved pace
	// floor doubles the number of turns.
	speed := max(settings.SpeedMinPercent, min(settings.SpeedMaxPercent, min(20, int(math.Round(float64(state.Target.SpeedPercent)*0.4)))))
	width := hi - lo
	narrow := min(width, max(20, width/2))
	prior := state.Perceptual
	// Keep the closer braking destination when it still turns less often.
	// A gesture keeps its span because changing both grammars and widths can
	// add a reversal near a fast turn.
	if prior.MeanStrokePercent > 0 && prior.CommandedMeanTravelPerSecond > 0 &&
		(state.Target.Flow == nil || state.Target.Flow.Gesture == nil) &&
		referenceTravelRateForSpeed(speed, settings.HandyModel)*prior.MeanStrokePercent < prior.CommandedMeanTravelPerSecond*float64(narrow) {
		width = narrow
	}
	start := lo + (hi-lo-width)/2
	flow := DefaultFlowSpec()
	flow.MinPercent, flow.MaxPercent = start, start+width
	flow.RangeFloorPercent, flow.RangeCeilingPercent = width, width
	flow.SpeedPercent, flow.PaceVariationPercent = speed, 0
	flow.AnchorPercent = 50
	return fitHoldingPace(flow, settings, prior)
}

func fitHoldingPace(flow FlowSpec, limits config.MotionSettings, prior PerceptualSummary) (MotionTarget, error) {
	for {
		target, err := flowTarget(flow, limits, "edge")
		if err != nil {
			return MotionTarget{}, err
		}
		target.Label = "Gentle hold"
		if prior.MeanStrokePercent <= 0 || prior.CommandedMeanTravelPerSecond <= 0 || prior.CommandedPeakVelocityPerSecond <= 0 {
			return target, nil
		}
		actual := NewMotionPlan("edge-fit", target, limits, 0, 0, time.Time{}).Perceptual
		turnRatio := actual.CommandedMeanTravelPerSecond * prior.MeanStrokePercent / (actual.MeanStrokePercent * prior.CommandedMeanTravelPerSecond)
		ratio := max(turnRatio, actual.CommandedPeakVelocityPerSecond/prior.CommandedPeakVelocityPerSecond)
		// Allow fitting and millisecond quantization at equal pace.
		if ratio <= 1.005 {
			return target, nil
		}
		if flow.SpeedPercent <= limits.SpeedMinPercent {
			return MotionTarget{}, errors.New("the saved minimum would make this hold faster; choose Hold still")
		}
		rate := referenceTravelRateForSpeed(flow.SpeedPercent, limits.HandyModel) / ratio
		flow.SpeedPercent = max(limits.SpeedMinPercent, min(flow.SpeedPercent-1, int(math.Floor(equivalentSpeedPercentForTravelRate(rate, limits.HandyModel)))))
	}
}
