//go:build magichandy_labs

// Review fixtures are parameter combinations, not runtime presets.
package main

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func renderCreativeV2Matrix() []motion.Review {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	entries := []motion.Review{}
	type scenario struct {
		name string
		edit func(*motion.FlowSpec)
	}
	cases := []scenario{
		{"Even full strokes", func(s *motion.FlowSpec) {
			s.Gesture.FocusMixPercent = 0
			s.Gesture.InertiaPercent = 0
			s.Gesture.VariationPercent = 0
		}},
		{"Irregular mixed excursions", func(s *motion.FlowSpec) {
			s.Gesture.FocusPercent = 50
			s.Gesture.VariationPercent = 75
			s.Gesture.FocusMixPercent = 55
		}},
		{"Fast tip sweeps with slow return", func(s *motion.FlowSpec) {
			s.Gesture.FocusMixPercent = 100
			s.Gesture.FasterDirection = "tip"
			s.Gesture.ContrastPercent = 65
		}},
		{"Fast base sweeps with slow return", func(s *motion.FlowSpec) {
			s.Gesture.FocusMixPercent = 100
			s.Gesture.FocusPercent = 0
			s.Gesture.FasterDirection = "base"
			s.Gesture.ContrastPercent = 65
		}},
		{"Middle work among full strokes", func(s *motion.FlowSpec) {
			s.Gesture.FocusPercent = 50
			s.Gesture.FocusMixPercent = 60
			s.Gesture.FocusWidthPercent = 25
		}},
		{"Base rebounds and full strokes", func(s *motion.FlowSpec) {
			s.Gesture.FocusPercent = 0
			s.Gesture.FocusWidthPercent = 45
			s.Gesture.FocusMixPercent = 55
			s.Gesture.ReboundCount = 3
			s.Gesture.ReboundDecayPercent = 75
			s.Gesture.InertiaPercent = 65
		}},
		{"Tip rebounds and full strokes", func(s *motion.FlowSpec) {
			s.Gesture.FocusPercent = 100
			s.Gesture.FocusWidthPercent = 45
			s.Gesture.FocusMixPercent = 55
			s.Gesture.ReboundCount = 3
			s.Gesture.ReboundDecayPercent = 75
			s.Gesture.InertiaPercent = 65
		}},
		{"Narrow band and truncated rebound tails", func(s *motion.FlowSpec) {
			s.MinPercent, s.MaxPercent = 30, 60
			s.Gesture.FocusWidthPercent = 15
			s.Gesture.FocusPercent = 0
			s.Gesture.ReboundCount = 4
			s.Gesture.ReboundDecayPercent = 85
			s.Gesture.InertiaPercent = 100
		}},
		{"Maximum combined character", func(s *motion.FlowSpec) {
			s.MinPercent, s.MaxPercent = 0, 100
			s.Gesture.FocusWidthPercent = 80
			s.Gesture.FocusMixPercent = 55
			s.Gesture.ReboundCount = 4
			s.Gesture.ReboundDecayPercent = 85
			s.Gesture.FasterDirection = "tip"
			s.Gesture.ContrastPercent = 80
			s.Gesture.InertiaPercent = 100
			s.Gesture.VariationPercent = 100
		}},
	}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, speed := range []int{10, 45, 85} {
			for _, sc := range cases {
				s := motion.DefaultFlowSpec()
				g := motion.DefaultGestureSpec()
				s.Gesture = &g
				s.RangeFloorPercent = 10
				s.SpeedPercent = speed
				sc.edit(&s)
				target, err := motion.FlowTarget(s, settings)
				if err != nil {
					panic(err)
				}
				entry := motion.ReviewMotionOutput(target, settings)
				entry.Name, entry.Group, entry.Description = sc.name, "creative-v2-matrix", "Generated from semantic parameters; no hardcoded preset path. Seed 17 for comparisons."
				entries = append(entries, entry)
			}
			for _, seed := range []uint32{17, 23681, 1763268511} {
				d := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{CenterPercent: 50, SpanPercent: 90, SpanMinPercent: 25, SpanProfile: "wander", VariationPercent: 65, PhraseSeed: seed})
				entry := motion.ReviewMotionOutput(motion.MotionTarget{Dynamic: &d, SpeedPercent: speed}, settings)
				entry.Name, entry.Group = fmt.Sprintf("Original Creative realization %d", seed), "creative-original"
				entries = append(entries, entry)
			}
		}
	}
	return entries
}
