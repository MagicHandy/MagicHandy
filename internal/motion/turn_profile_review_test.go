//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Inert alternatives for physical review. They reuse semantic legs, exact
// original clocks/joint accelerations, the safety envelope and shared sampler.
// This fixture does not change the production generator or open a transport.
func TestLayeredTurnProfileReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_TURN_PROFILE_REVIEW")
	if path == "" {
		t.Skip("set the ignored inert review output path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, settings.HandyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
	var entries []Review
	for _, speed := range []int{10, 25, 58, 90} {
		for _, width := range []int{28, 100, 0} {
			for _, rate := range []int{100, 25, 12} {
				spec := DefaultFlowSpec()
				spec.MinPercent, spec.MaxPercent, spec.RangeFloorPercent, spec.RangeCeilingPercent = 0, 100, width, width
				spec.SpeedPercent, spec.PaceVariationPercent, spec.LoopCycles = speed, 0, 6
				var legs []gestureLeg
				var durations []int64
				if rate != 100 || width == 0 {
					if width == 0 {
						spec.RangeFloorPercent, spec.RangeCeilingPercent = 28, 100
						spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: rate}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: rate}}}
					} else {
						spec.MinPercent = 100 - width
						spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{{BottomPercent: 100 - width, TopPercent: 100, Count: 1, RatePercent: rate}}}
					}
					legs, durations = phraseLegs(spec, settings.HandyModel)
				} else {
					legs, durations = flowLegs(spec, settings.HandyModel)
				}
				curves, err := fitLegCurves(legs, durations, budget)
				if err != nil {
					t.Fatal(err)
				}
				for _, ramp := range []float64{0, .28, .18, .10} {
					selected := slices.Clone(curves)
					accepted := 0
					for i, leg := range legs {
						if ramp == 0 {
							continue
						}
						original := curves[i]
						candidate := reviewCruiseLeg(leg, durations[i], original.accelerations[0], original.accelerations[len(original.accelerations)-1], ramp)
						if legTimeScale(candidate, budget) <= 1 {
							selected[i], accepted = candidate, accepted+1
						}
					}
					curve, err := assembleLegCurve(legs, selected, "Layered turn review")
					if err != nil {
						t.Fatal(err)
					}
					target := MotionTarget{SpeedPercent: speed, Flow: &spec, prepared: &preparedMotion{id: "turn-review", name: "Turn review", curve: curve,
						referenceRate: referenceTravelRateForSpeed(speed, settings.HandyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}}
					entry := ReviewMotionOutput(target, settings)
					entry.ID = fmt.Sprintf("w%d-s%d-rate%d-r%.2f", width, speed, rate, ramp)
					entry.Name, entry.Group = entry.ID, "turn-profile-review"
					entry.Description = fmt.Sprintf("Inert C2 cruise ramp %.2f, accepted %d/%d legs at unchanged durations; no physical validation", ramp, accepted, len(legs))
					entries = append(entries, entry)
				}
			}
		}
	}
	encoded, err := json.Marshal(map[string]any{"entries": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

func reviewCruiseLeg(leg gestureLeg, duration int64, start, end, ramp float64) Curve {
	times := []int64{0, duration, int64(math.Round(ramp * float64(duration))), int64(math.Round((1 - ramp) * float64(duration)))}
	for i := 1; i < 8; i++ {
		times = append(times, int64(math.Round(float64(duration)*float64(i)/8)))
	}
	slices.Sort(times)
	times = slices.Compact(times)
	points := make([]CurvePoint, len(times))
	velocities, accelerations := make([]float64, len(times)), make([]float64, len(times))
	distance := leg.to - leg.from
	start *= float64(duration*duration) / distance
	end *= float64(duration*duration) / distance
	for i, at := range times {
		x, v, a := jointCruiseProgress(float64(at)/float64(duration), ramp, ramp, start, end)
		points[i] = CurvePoint{TimeMillis: at, PositionPercent: leg.from + distance*x}
		velocities[i], accelerations[i] = distance*v/float64(duration), distance*a/float64(duration*duration)
	}
	points[0].PositionPercent, points[len(points)-1].PositionPercent = leg.from, leg.to
	return Curve{points: points, slopes: velocities, accelerations: accelerations, quintics: buildQuinticSegments(points, velocities, accelerations), duration: duration}
}
