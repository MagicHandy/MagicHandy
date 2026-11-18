//go:build magichandy_labs

package main

import (
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"reflect"
)

func initialScore(method string, limits config.MotionSettings) motion.FlowSpec {
	current := chat.FreshCreativeV2Score(25)
	if method == "layered" {
		current = chat.FreshLayeredScore(25)
	}
	if chat.IsStrokeLabMethod(method) {
		current = chat.FreshStrokeLabScore(limits)
	}
	if method == "sequence" {
		current = motion.DefaultFlowSpec()
	}
	if method == "adaptive" {
		current = motion.NewFreestyleFlow(159261, 25, motion.FreestyleSpec{Strokes: 64, MinSpeedPercent: 10, Keyframes: []motion.FreestyleKeyframe{{Controls: motion.FreestyleControls{PacePercent: 50, LengthPercent: 65, FocusPercent: 50, RoamingPercent: 50, VarietyPercent: 60, EnergyPercent: 100}}}})
	}
	current.Seed = 159261
	return current
}
func reviewCases(phase, method string) []string {
	cases := []string{"Use only full strokes across the whole range, with equal timing in each direction, no rebounds and no inertia. Keep the speed setting."}
	if phase == "suite" {
		cases = []string{"Stay in the shallow upper quarter of the range. Keep the speed setting and travel timing even in both directions.", "Make the lengths vary naturally, sometimes shorter, without going any deeper or changing speed. No repeating buildup or one-direction accent.", "Keep this exact score unchanged from now on.", "Explain what the motion preview estimates. Do not change anything."}
		if method == "sequence" {
			cases = []string{"Repeat four full-range cycles at speed 25, then two upper-quarter cycles at speed 25, then four full-range cycles at speed 25. Make this exact three-section sequence.", "Reduce every section's speed by exactly five points. Preserve the ranges, cycles and ordering.", "Keep this exact score unchanged from now on.", "Explain what the motion preview estimates. Do not change anything."}
		}
	}
	if phase == "steering" {
		cases = []string{"Stay in the shallow upper quarter of the range. Keep the speed setting and travel timing even in both directions.", "Make the lengths vary naturally, sometimes shorter, without going any deeper or changing speed. No repeating buildup or one-direction accent.", "Keep this exact score unchanged from now on.", "Explain what the motion preview estimates. Do not change anything.", "Now confine all motion to the deepest quarter of the slider, with changing stroke lengths. Keep the speed.", "Now stay entirely between 35 and 65, with shorter strokes mixed in. Leave the pace setting alone.", "Return to full strokes across the entire slider, with equal travel timing and no rebounds or inertia.", "Keep this exact score unchanged from now on."}
	}
	return cases
}
func reviewIntent(phase, method string, index int, trial chat.LLMLabTrial, current motion.FlowSpec, summary motion.PerceptualSummary) bool {
	if !trial.Valid {
		return false
	}
	if phase == "screen" {
		return evenFullReviewStroke(trial.After, current.SpeedPercent)
	}
	if method == "sequence" {
		if index >= 2 {
			return reflect.DeepEqual(current, trial.After)
		}
		want := 25 - index*5
		expected := []motion.FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: want, Cycles: 4}, {MinPercent: 75, MaxPercent: 100, SpeedPercent: want, Cycles: 2}, {MinPercent: 0, MaxPercent: 100, SpeedPercent: want, Cycles: 4}}
		return reflect.DeepEqual(trial.After.Steps, expected) && summary.MeanStrokePercent >= 75
	}
	switch index {
	case 0, 1:
		return insideReviewBand(summary, 72, 100) && trial.After.SpeedPercent == current.SpeedPercent && (index == 0 || summary.StrokeLengthCV > 0.03)
	case 2, 3, 7:
		return reflect.DeepEqual(current, trial.After)
	case 4:
		return insideReviewBand(summary, 0, 25.1) && trial.After.SpeedPercent == current.SpeedPercent
	case 5:
		return insideReviewBand(summary, 34.9, 65.1) && trial.After.SpeedPercent == current.SpeedPercent
	case 6:
		return evenFullReviewStroke(trial.After, current.SpeedPercent)
	default:
		return true
	}
}

func evenFullReviewStroke(after motion.FlowSpec, speed int) bool {
	g := after.Gesture
	return g != nil && after.MinPercent == 0 && after.MaxPercent == 100 && g.FocusMixPercent == 0 && g.InertiaPercent == 0 && g.ReboundCount == 0 && g.FasterDirection == "even" && after.SpeedPercent == speed
}
func insideReviewBand(summary motion.PerceptualSummary, lo, hi float64) bool {
	return summary.PositionMinPercent >= lo && summary.PositionMaxPercent <= hi
}
