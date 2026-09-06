//go:build magichandy_labs

package main

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// These authored fixtures cover timing contrast, subtle motion, chatter around
// a major reversal, intentional dwell, and windows too short to round.
func renderMediaFilters() []motion.Review {
	cases := []struct {
		name    string
		actions []media.FunscriptAction
	}{
		{"wide", []media.FunscriptAction{{AtMillis: 0, Position: 10}, {AtMillis: 1000, Position: 90}, {AtMillis: 2000, Position: 10}, {AtMillis: 3000, Position: 90}, {AtMillis: 4000, Position: 10}}},
		{"asymmetric", []media.FunscriptAction{{AtMillis: 0, Position: 10}, {AtMillis: 400, Position: 90}, {AtMillis: 2000, Position: 10}, {AtMillis: 2400, Position: 90}, {AtMillis: 4000, Position: 10}}},
		{"subtle", []media.FunscriptAction{{AtMillis: 0, Position: 48}, {AtMillis: 800, Position: 52}, {AtMillis: 1600, Position: 48}, {AtMillis: 2400, Position: 52}, {AtMillis: 3200, Position: 48}}},
		{"chatter", []media.FunscriptAction{{AtMillis: 0, Position: 10}, {AtMillis: 1000, Position: 90}, {AtMillis: 1040, Position: 88}, {AtMillis: 1080, Position: 90}, {AtMillis: 2200, Position: 10}, {AtMillis: 3200, Position: 90}, {AtMillis: 4000, Position: 10}}},
		{"dwell", []media.FunscriptAction{{AtMillis: 0, Position: 10}, {AtMillis: 800, Position: 90}, {AtMillis: 1200, Position: 90}, {AtMillis: 2000, Position: 10}, {AtMillis: 2400, Position: 10}, {AtMillis: 3200, Position: 90}, {AtMillis: 4000, Position: 10}}},
		{"dense", []media.FunscriptAction{{AtMillis: 0, Position: 48}, {AtMillis: 8, Position: 52}, {AtMillis: 16, Position: 48}, {AtMillis: 24, Position: 52}, {AtMillis: 32, Position: 48}, {AtMillis: 1000, Position: 52}, {AtMillis: 2000, Position: 48}}},
	}
	variants := []struct {
		name    string
		filters media.Filters
		limit   bool
	}{
		{name: "authored"}, {name: "smoothing", filters: media.Filters{SmoothingPercent: 3}},
		{name: "rounding", filters: media.Filters{PeakRoundingMillis: 60}},
		{name: "combined_limited", filters: media.Filters{SmoothingPercent: 3, PeakRoundingMillis: 200}, limit: true},
	}
	entries := []motion.Review{}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings := config.DefaultSettings().Motion
		settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = model, 1, 40
		for _, test := range cases {
			for _, rate := range []float64{0.5, 1, 2} {
				for _, variant := range variants {
					script := media.Funscript{VideoID: test.name, Name: test.name, Actions: test.actions, DurationMillis: test.actions[len(test.actions)-1].AtMillis}
					timeline, effect, err := script.TimelineFrom(0, rate, variant.filters)
					must(err)
					settings.ApplyVideoSpeedLimit = variant.limit
					entry := motion.ReviewMotionOutput(motion.MotionTarget{Source: motion.TargetSourceMedia, Label: test.name, Media: &timeline}, settings)
					entry.ID = fmt.Sprintf("%s-%s-%.1fx-%s", test.name, variant.name, rate, model)
					entry.Name, entry.Group = entry.ID, "media-filters"
					entry.Description = fmt.Sprintf("Authored clock %.1fx; smoothing %d%%, rounding %dms, speed cap %t at 40%%; %d actions removed. Commanded estimates only.", rate, variant.filters.SmoothingPercent, variant.filters.PeakRoundingMillis, variant.limit, effect.ActionsRemoved)
					entries = append(entries, entry)
				}
			}
		}
	}
	return entries
}
