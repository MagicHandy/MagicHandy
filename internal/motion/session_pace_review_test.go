//go:build magichandy_labs

package motion

import (
	"fmt"
	"os"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Inert replay of numeric score geometry captured from the October 31 report.
// Seeds were not retained in the chat prompt; multiple fixed seeds expose that
// uncertainty instead of presenting a guessed seed as the exact live pattern.
func TestSessionPaceReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SESSION_PACE_REVIEW")
	if path == "" {
		t.Skip("set MAGICHANDY_SESSION_PACE_REVIEW for an inert atlas")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	var entries []Review
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		limits.HandyModel = model
		for _, speed := range []int{25, 60, 78, 100} {
			for _, seed := range []uint32{17, 29, 113} {
				for _, name := range []string{"captured-alternate", "region-transfer", "full-directional"} {
					s := layeredSpeedFixture("full", speed)
					s.Seed = seed
					s.DirectionalPace = &DirectionalPace{85, 100}
					switch name {
					case "captured-alternate":
						s.LoopCycles = 64
						s.MinPercent, s.MaxPercent, s.AnchorPercent = 35, 100, 60
						s.RangeFloorPercent, s.RangeCeilingPercent = 18, 30
						s.MemoryCycles, s.PaceVariationPercent, s.VariationMode = 8, 8, "waves"
						s.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 75, PeriodCycles: 8, Shape: "alternate", LowRatePercent: 55}, {Axis: "center", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"}}
					case "region-transfer":
						s.Phrase = &FlowPhrase{Regions: []PhraseRegion{{PhraseStroke{70, 100, 3, 100}, "down_up"}, {PhraseStroke{0, 30, 3, 100}, "up_down"}}}
					}
					target, err := FlowTarget(s, limits)
					if err != nil {
						t.Fatal(err)
					}
					entry := reviewMotionOutput(target, limits, func(e *Engine) { configurePathReview(e, "cloud") })
					entry.ID = fmt.Sprintf("%s-%s-%d-%d", name, model, speed, seed)
					entry.Name, entry.Group = entry.ID, model
					entries = append(entries, entry)
					if entry.Error != "" {
						t.Errorf("%s: %s", entry.ID, entry.Error)
					}
					t.Logf("%s effective=%.2f mean=%.2f", entry.ID, entry.Summary.Pace.EffectivePercent, entry.Summary.CommandedMeanTravelPerSecond)
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}
