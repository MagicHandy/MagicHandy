//go:build magichandy_labs

package motion

import (
	"fmt"
	"math"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

type followupJoinReview struct {
	Name            string       `json:"name"`
	Phase           float64      `json:"phase"`
	Splice          bool         `json:"splice"`
	ComputeMicros   int64        `json:"compute_us"`
	DelayMillis     int64        `json:"delay_ms"`
	DurationMillis  int64        `json:"duration_ms"`
	Acceleration    float64      `json:"acceleration"`
	Jerk            float64      `json:"jerk"`
	OldAcceleration float64      `json:"old_acceleration"`
	NewAcceleration float64      `json:"new_acceleration"`
	Samples         []CurvePoint `json:"samples"`
}

// Reproduces the independent review's concerns without constructing a real
// transport. Each report retains failed joins and the complete sampled path.
func TestLayeredFollowupReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LAYERED_FOLLOWUP_REPORT")
	if path == "" {
		t.Skip("set an ignored follow-up report path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	var mixed []map[string]any
	for _, geometry := range []string{"compound", "equal-full", "wide-short"} {
		for _, rate := range []int{5, 15, 35, 65, 100} {
			spec := layeredSpeedFixture("compound", 100)
			if geometry == "equal-full" {
				spec.Phrase.Strokes[0].BottomPercent = 0
			}
			if geometry == "wide-short" {
				spec.Phrase.Strokes[0].BottomPercent = 40
			}
			spec.Phrase.Strokes[1].RatePercent = rate
			target := followupTarget(t, spec, settings)
			entry := speedSupportCloudReview(target, settings)
			entry.ID, entry.Name, entry.Group = fmt.Sprintf("%s-%d", geometry, rate), fmt.Sprintf("%s: slow sweep at %d%% relative rate", geometry, rate), "mixed-rate"
			entries = append(entries, entry)
			legs, desired := phraseLegs(spec, settings.HandyModel)
			fitted := slices.Clone(desired)
			original, err := fitLegCurves(legs, fitted, legBudget{velocity: referenceTravelRateForSpeed(100, settings.HandyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget})
			if err != nil {
				t.Fatal(err)
			}
			curve := target.prepared.curve
			deviation, count := 0., 0
			for i, leg := range original {
				if fitted[i] != desired[i] {
					continue
				}
				count++
				start, end := curve.authoredKnots[i].TimeMillis, curve.authoredKnots[i+1].TimeMillis
				for j := 0; j <= 100; j++ {
					u := float64(j) / 100
					deviation = max(deviation, math.Abs(curve.sampleFloat(float64(start)+u*float64(end-start))-leg.sampleFloat(u*float64(leg.duration))))
				}
			}
			mixed = append(mixed, map[string]any{"geometry": geometry, "rate": rate, "unconstrained_legs": count, "maximum_shape_change": deviation, "period_ms": curve.duration})
		}
	}
	for _, variation := range []int{0, 10, 25, 40} {
		spec := layeredSpeedFixture("full", 10)
		spec.PaceVariationPercent = variation
		entry := speedSupportCloudReview(followupTarget(t, spec, settings), settings)
		entry.ID, entry.Name, entry.Group = fmt.Sprintf("variation-%d", variation), fmt.Sprintf("Deliberate pace variation %d%%", variation), "pace-reporting"
		entries = append(entries, entry)
	}
	var joins []followupJoinReview
	for _, shape := range []string{"full", "short", "compound"} {
		for _, speed := range []int{1, 5, 25, 100} {
			before := followupTarget(t, layeredSpeedFixture(shape, speed), settings)
			hold, err := HoldingTarget(ActiveMotionState{Target: before}, settings)
			if err != nil {
				t.Fatal(err)
			}
			pairs := map[string]MotionTarget{"edge": hold}
			if speed == 100 {
				pairs["drop-1"] = followupTarget(t, layeredSpeedFixture(shape, 1), settings)
				pairs["drop-5"] = followupTarget(t, layeredSpeedFixture(shape, 5), settings)
			}
			for name, after := range pairs {
				old := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
				for _, phase := range []float64{.01, .07, .17, .31, .43, .57, .71, .83, .93, .999} {
					at := int64(float64(old.PeriodMillis) * phase)
					next := old.Retarget("after", after, settings, at, time.Unix(0, 0))
					started := time.Now()
					bridge := layeredTurnTransition(old, nil, &next, at)
					cost := time.Since(started).Microseconds()
					spliced := bridge != nil
					if bridge == nil {
						bridge = newPlanTransition(old, nil, at)
					}
					row := followupJoinReview{Name: fmt.Sprintf("%s-%d-%s", shape, speed, name), Phase: phase, Splice: spliced, ComputeMicros: cost,
						DelayMillis: bridge.strokeStartMillis - at, DurationMillis: bridge.endMillis - at,
						OldAcceleration: maximumPlanAcceleration(old), NewAcceleration: maximumPlanAcceleration(next)}
					if spliced {
						row.Acceleration = bridge.stroke.maximumAcceleration() * 1e6
						row.Jerk = bridge.stroke.maximumJerk() * 1e9
					}
					for clock := max(int64(0), at-300); clock <= bridge.endMillis+500; clock += 2 {
						row.Samples = append(row.Samples, CurvePoint{TimeMillis: clock - at, PositionPercent: sampleMotionPath(next, bridge, clock).PositionPercent})
					}
					joins = append(joins, row)
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries, "mixed": mixed, "joins": joins})
}

func followupTarget(t *testing.T, spec FlowSpec, settings config.MotionSettings) MotionTarget {
	t.Helper()
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestLayeredRapidFollowupReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LAYERED_RAPID_REPORT")
	if path == "" {
		t.Skip("set an ignored rapid-retarget report path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	old := NewMotionPlan("before", followupTarget(t, layeredSpeedFixture("full", 35), settings), settings, 0, 0, time.Unix(0, 0))
	first := old.Retarget("first", followupTarget(t, layeredSpeedFixture("full", 85), settings), settings, 1200, time.Unix(0, 0))
	history := layeredTurnTransition(old, nil, &first, 1200)
	if history == nil {
		t.Fatal("fixture needs a first bridge")
	}
	at := history.endMillis + 350
	next := first.Retarget("gentle", followupTarget(t, layeredSpeedFixture("full", 7), settings), settings, at, time.Unix(0, 0))
	legacyNext := next
	// Alpha.75 retained the still-playing history and fell back at this future
	// handoff. Preserve that exact crossfade as the comparison, not a surrogate.
	legacy := newPlanTransition(first, history, at)
	bridge := layeredTurnTransition(first, history, &next, at)
	if bridge == nil {
		t.Fatal("completed history forced a crossfade")
	}
	rows := map[string][]CurvePoint{"alpha75": {}, "candidate": {}}
	for clock := int64(900); clock <= max(bridge.endMillis, legacy.endMillis)+1200; clock += 2 {
		rows["alpha75"] = append(rows["alpha75"], CurvePoint{TimeMillis: clock, PositionPercent: sampleMotionPath(legacyNext, legacy, clock).PositionPercent})
		rows["candidate"] = append(rows["candidate"], CurvePoint{TimeMillis: clock, PositionPercent: sampleMotionPath(next, bridge, clock).PositionPercent})
	}
	writeSpeedReviewReport(t, path, map[string]any{"handoff_ms": at, "first_handoff_ms": 1200, "samples": rows})
}
