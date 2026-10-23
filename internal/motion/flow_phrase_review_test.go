//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Export the actual shared handoff path, without a transport or playback loop.
func TestPhraseHandoffVisualReview(t *testing.T) {
	output := os.Getenv("MAGICHANDY_PHRASE_HANDOFF_OUTPUT")
	if output == "" {
		t.Skip("set the inert phrase handoff output path")
	}
	var rows []map[string]any
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, speed := range []int{7, 46, 100} {
			for _, anchor := range []int{0, 100} {
				settings := config.DefaultSettings().Motion
				settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = profile, 1, speed
				s := phraseFixture(anchor, 70, 2, speed)
				if anchor == 100 {
					s.Phrase.Strokes[0].BottomPercent = 30
				}
				s.PaceVariationPercent = 0
				a, err := FlowTarget(s, settings)
				if err != nil {
					t.Fatal(err)
				}
				old := NewMotionPlan("old", a, settings, 0, 0, time.Unix(0, 0))
				if anchor == 100 {
					s.Phrase.Strokes[0].BottomPercent = 84
				} else {
					s.Phrase.Strokes[0].TopPercent = 16
				}
				b, err := FlowTarget(s, settings)
				if err != nil {
					t.Fatal(err)
				}
				leg := 4
				if anchor == 100 {
					leg = 3
				}
				knots := old.curve.authoredKnots
				at := int64((float64(knots[leg-1].TimeMillis) + .4*float64(knots[leg].TimeMillis-knots[leg-1].TimeMillis)) * float64(old.PeriodMillis) / float64(old.curve.duration))
				next := old.Retarget("new", b, settings, at, time.Unix(0, 0))
				bridge := layeredPaceTransition(old, nil, &next, at)
				if bridge == nil {
					t.Fatal("missing bounded join")
				}
				var path [][3]float64
				for now := max(int64(0), at-300); now <= bridge.endMillis+1000; now += 5 {
					x := sampleMotionPath(next, bridge, now).PositionPercent
					v := (sampleMotionPath(next, bridge, now+1).PositionPercent - sampleMotionPath(next, bridge, now-1).PositionPercent) * 500
					path = append(path, [3]float64{float64(now-at) / 1000, x, v})
				}
				rows = append(rows, map[string]any{"name": fmt.Sprintf("%s / %d%% / anchor%d", profile, speed, anchor), "join_start": float64(bridge.strokeStartMillis-at) / 1000, "join_end": float64(bridge.endMillis-at) / 1000, "path": path})
			}
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
}
