//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// This opt-in probe uses inert plans and the production sampler, not a device.
func TestLayeredSmoothnessProbe(t *testing.T) {
	input := os.Getenv("MAGICHANDY_LAYERED_SMOOTHNESS_INPUT")
	output := os.Getenv("MAGICHANDY_LAYERED_SMOOTHNESS_OUTPUT")
	if output == "" {
		t.Skip("set the inert comparison output path; optional input overrides the slow-motion matrix")
	}
	cases := slowMotionReviewCases()
	if input != "" {
		data, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &cases); err != nil {
			t.Fatal(err)
		}
	}
	var results []map[string]any
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, row := range cases {
		profiles := []string{settings.HandyModel}
		if os.Getenv("MAGICHANDY_SMOOTHNESS_FULL_MATRIX") == "1" {
			profiles = []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro}
		}
		for _, profile := range profiles {
			settings.HandyModel = profile
			target, err := FlowTarget(row.Spec, settings)
			if err != nil {
				t.Fatal(row.Name, err)
			}
			plan := NewMotionPlan(row.Name, target, settings, 0, 0, time.Unix(0, 0))
			knots := plan.curve.authoredKnots
			minimumSpan := 100.0
			var tinyMillis int64
			for i := 1; i < len(knots); i++ {
				span := math.Abs(knots[i].PositionPercent - knots[i-1].PositionPercent)
				minimumSpan = math.Min(minimumSpan, span)
				if span < 1 {
					tinyMillis += knots[i].TimeMillis - knots[i-1].TimeMillis
				}
			}
			engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
				preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 100}
			var samples []MotionSample
			end := min(plan.PeriodMillis, int64(20000))
			if os.Getenv("MAGICHANDY_SMOOTHNESS_FULL_MATRIX") == "1" {
				end = plan.PeriodMillis + 2000
			}
			for engine.nextSampleMillis < end {
				chunk, err := engine.nextMotionSamplesLocked()
				if err != nil {
					t.Fatal(row.Name, err)
				}
				samples = append(samples, chunk...)
			}
			var squaredError, squaredVelocityError, instantaneousError, maximumError, excessVariation, previousWireVelocity, previousPlanVelocity float64
			var velocitySteps []float64
			var stationary int64
			var wire [][3]float64
			for i := 1; i < len(samples); i++ {
				left, right := samples[i-1], samples[i]
				duration := float64(right.TimeMillis - left.TimeMillis)
				l, r := math.Round(left.PositionPercent), math.Round(right.PositionPercent)
				wv := (r - l) * 1000 / duration
				pv := (plan.SampleAt(right.TimeMillis).PositionPercent - plan.SampleAt(left.TimeMillis).PositionPercent) * 1000 / duration
				if i > 1 && wv*previousWireVelocity > 0 {
					excessVariation += math.Max(0, math.Abs(wv-previousWireVelocity)-math.Abs(pv-previousPlanVelocity))
				}
				if i > 1 {
					velocitySteps = append(velocitySteps, math.Abs(wv-previousWireVelocity))
				}
				squaredVelocityError += (wv - pv) * (wv - pv) * duration
				previousWireVelocity, previousPlanVelocity = wv, pv
				if l == r {
					stationary += int64(duration)
				}
				wire = append(wire, [3]float64{float64(right.TimeMillis), r, wv})
				for at := left.TimeMillis; at < right.TimeMillis; at++ {
					x := l + (r-l)*float64(at-left.TimeMillis)/duration
					e := x - plan.SampleAt(at).PositionPercent
					squaredError += e * e
					maximumError = math.Max(maximumError, math.Abs(e))
					clock := plan.PhaseAt(at) * float64(plan.curve.duration)
					velocity := plan.curve.velocityFloat(clock) * plan.focus.gain() * float64(plan.curve.duration) / float64(plan.PeriodMillis) * 1000
					instantaneousError += (wv - velocity) * (wv - velocity)
				}
			}
			duration := float64(samples[len(samples)-1].TimeMillis - samples[0].TimeMillis)
			sort.Float64s(velocitySteps)
			travel := 0.0
			for i := 1; i < len(knots); i++ {
				travel += math.Abs(knots[i].PositionPercent - knots[i-1].PositionPercent)
			}
			invariance := 0.0
			if strings.HasPrefix(row.Name, "steady-") {
				invariance = smoothnessStrokeVariation(plan, samples)
			}
			results = append(results, map[string]any{"name": row.Name, "model": profile, "spec": row.Spec, "period_ms": plan.PeriodMillis,
				"minimum_span": minimumSpan, "tiny_stroke_ms": tinyMillis, "stationary_ms": stationary,
				"travel": travel, "mean_rate": travel * 1000 / float64(plan.PeriodMillis), "instantaneous_velocity_error_rms": math.Sqrt(instantaneousError / duration),
				"velocity_step_p95": velocitySteps[int(float64(len(velocitySteps)-1)*.95)], "velocity_step_max": velocitySteps[len(velocitySteps)-1], "stroke_variation_rms": invariance,
				"wire_error_max": maximumError, "wire_error_rms": math.Sqrt(squaredError / duration),
				"velocity_error_rms": math.Sqrt(squaredVelocityError / duration), "excess_variation_per_second": excessVariation * 1000 / duration,
				"points": len(samples), "wire": wire, "knots": knots})
		}
	}
	encoded, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

type smoothnessReviewCase struct {
	Name string
	Spec FlowSpec
}

func slowMotionReviewCases() []smoothnessReviewCase {
	var cases []smoothnessReviewCase
	for _, speed := range []int{10, 25, 58, 90} {
		base := FlowSpec{MinPercent: 0, MaxPercent: 100, SpeedPercent: speed,
			AnchorPercent: 100, MemoryCycles: 8, Seed: 17, LoopCycles: 6}
		for _, width := range []int{10, 28, 100} {
			spec := base
			spec.RangeFloorPercent, spec.RangeCeilingPercent = width, width
			cases = append(cases, smoothnessReviewCase{fmt.Sprintf("steady-w%d-s%d", width, speed), spec})
		}
		for _, rate := range []int{25, 12} {
			spec := base
			spec.RangeFloorPercent, spec.RangeCeilingPercent = 28, 100
			spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
				{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: rate},
				{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: rate},
			}}
			name := fmt.Sprintf("phrase-slow-s%d", speed)
			if rate == 12 {
				name += "-rate12"
			}
			cases = append(cases, smoothnessReviewCase{name, spec})
		}
	}
	return cases
}

// Compare wire velocities at the same normalized phase of identical strokes.
// Their planned variation is zero; this measures variation introduced by the
// sampler's probe grid and append partitions, independently of crossing error.
func smoothnessStrokeVariation(plan MotionPlan, samples []MotionSample) float64 {
	var sum, square [32]float64
	count := 0.0
	knots := plan.curve.authoredKnots
	for leg := 0; leg < len(knots)-1; leg++ {
		left, right := knots[leg].TimeMillis, knots[leg+1].TimeMillis
		if right > samples[len(samples)-1].TimeMillis {
			break
		}
		for phase := range 32 {
			at := float64(left) + float64(right-left)*(float64(phase)+.5)/32
			index := sort.Search(len(samples), func(i int) bool { return float64(samples[i].TimeMillis) > at })
			if index <= 0 || index >= len(samples) {
				continue
			}
			a, b := samples[index-1], samples[index]
			velocity := math.Abs(math.Round(b.PositionPercent)-math.Round(a.PositionPercent)) * 1000 / float64(b.TimeMillis-a.TimeMillis)
			sum[phase] += velocity
			square[phase] += velocity * velocity
		}
		count++
	}
	if count == 0 {
		return 0
	}
	variance := 0.0
	for i := range 32 {
		variance += math.Max(0, square[i]/count-math.Pow(sum[i]/count, 2))
	}
	return math.Sqrt(variance / 32)
}
