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
	intent := trial.Valid
	if phase == "screen" {
		intent = intent && trial.After.MinPercent == 0 && trial.After.MaxPercent == 100 && trial.After.Gesture.FocusMixPercent == 0 && trial.After.Gesture.InertiaPercent == 0 && trial.After.Gesture.ReboundCount == 0 && trial.After.Gesture.FasterDirection == "even" && trial.After.SpeedPercent == current.SpeedPercent
	} else if method == "sequence" {
		if index < 2 {
			want := 25 - index*5
			expected := []motion.FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: want, Cycles: 4}, {MinPercent: 75, MaxPercent: 100, SpeedPercent: want, Cycles: 2}, {MinPercent: 0, MaxPercent: 100, SpeedPercent: want, Cycles: 4}}
			intent = intent && reflect.DeepEqual(trial.After.Steps, expected) && summary.MeanStrokePercent >= 75
		} else {
			intent = intent && reflect.DeepEqual(current, trial.After)
		}
	} else {
		switch index {
		case 0:
			intent = intent && summary.PositionMinPercent >= 72 && summary.PositionMaxPercent <= 100 && trial.After.SpeedPercent == current.SpeedPercent
		case 1:
			intent = intent && summary.PositionMinPercent >= 72 && summary.PositionMaxPercent <= 100 && trial.After.SpeedPercent == current.SpeedPercent && summary.StrokeLengthCV > 0.03
		case 2, 3, 7:
			intent = intent && reflect.DeepEqual(current, trial.After)
		case 4:
			intent = intent && summary.PositionMinPercent >= 0 && summary.PositionMaxPercent <= 25.1 && trial.After.SpeedPercent == current.SpeedPercent
		case 5:
			intent = intent && summary.PositionMinPercent >= 34.9 && summary.PositionMaxPercent <= 65.1 && trial.After.SpeedPercent == current.SpeedPercent
		case 6:
			intent = intent && trial.After.MinPercent == 0 && trial.After.MaxPercent == 100 && trial.After.Gesture.FocusMixPercent == 0 && trial.After.Gesture.InertiaPercent == 0 && trial.After.Gesture.ReboundCount == 0 && trial.After.Gesture.FasterDirection == "even" && trial.After.SpeedPercent == current.SpeedPercent
		}
	}
	return intent
}
