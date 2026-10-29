//go:build magichandy_labs

package motion

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// TestMotionPathReview is an inert end-to-end comparison of authored intent,
// compiled trajectories and emitted points. Experimental source overlays use
// this same matrix; every failure remains in the report. It creates no device.
func TestMotionPathReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_PATH_REVIEW")
	if path == "" {
		t.Skip("set MAGICHANDY_PATH_REVIEW to an ignored atlas path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	var metrics []motionPathMetrics
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		speeds := []int{25, 75, 85, 100}
		if model == config.HandyModelOriginal {
			speeds = []int{5, 25, 60, 75, 85, 92, 100}
		}
		for _, speed := range speeds {
			fixtures := motionPathFixtures(speed)
			var names []string
			for name := range fixtures {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				spec := fixtures[name]
				target, err := FlowTarget(spec, settings)
				policies := []string{"cloud"}
				if (speed == 25 || speed == 100) && (name == "full" || name == "short" || name == "compound" || name == "regional" || name == "creative-v2") {
					policies = append(policies, "immediate-50", "intiface")
				}
				if os.Getenv("MAGICHANDY_PATH_REVIEW_TRANSPORT") == "ble" {
					policies = []string{"ble"}
				}
				for _, policy := range policies {
					id := fmt.Sprintf("%s-%s-%03d-%s", model, name, speed, policy)
					var entry Review
					if err != nil {
						entry.Error = err.Error()
					} else {
						entry = reviewMotionOutput(target, settings, func(e *Engine) { configurePathReview(e, policy) })
					}
					entry.ID, entry.Name, entry.Group = id, id, policy
					entry.Description = "Shared compiler and transport sampling policy. Wire is commanded output, not carriage telemetry."
					metric := motionPathMetrics{ID: id}
					if err == nil {
						metric = measureMotionPath(entry, target, settings, policy)
					}
					entries, metrics = append(entries, entry), append(metrics, metric)
					if entry.Error != "" {
						t.Errorf("%s: %s", id, entry.Error)
					}
					if metric.PeakWireVelocity > referenceTravelRateForSpeed(100, model)*1.001 || metric.MissingTurns > 0 {
						t.Errorf("%s: wire velocity %.3f; missing turns %d", id, metric.PeakWireVelocity, metric.MissingTurns)
					}
					if (policy == "cloud" || policy == "ble") && metric.MaximumWireError > metric.PositionTolerance+.5001 {
						t.Errorf("%s: path error %.4f exceeds %.4f", id, metric.MaximumWireError, metric.PositionTolerance+.5)
					}
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries, "metrics": metrics})
	t.Logf("retained %d trajectories and transport results in %s", len(entries), path)
}

func configurePathReview(e *Engine, policy string) {
	if policy == "cloud" || policy == "ble" {
		caps := (&transport.CloudRESTTransport{}).MotionSamplingCapabilities()
		if policy == "ble" {
			caps = (&transport.HandyBluetoothTransport{}).MotionSamplingCapabilities()
		}
		e.preferSparseGeneratedMotion = caps.PreferSparseGeneratedMotion
		e.positionResolutionPercent = caps.PositionResolutionPercent
		e.maximumChunkPoints = caps.MaximumPointsPerAppend
	} else {
		e.positionResolutionPercent = 0
		e.minimumPointIntervalMillis, e.preferredPointIntervalMillis = 50, 110
		if policy == "intiface" {
			e.minimumPointIntervalMillis = 110
		}
	}
}

func motionPathFixtures(speed int) map[string]FlowSpec {
	fixtures := layeredReleaseFixtures(speed)
	fixtures["regional"], fixtures["regional-mirror"] = regionalPhraseFixture(speed, false), regionalPhraseFixture(speed, true)
	fixtures["down-fast"], fixtures["up-fast"] = directionalReviewSpec("compound", speed, false), directionalReviewSpec("compound", speed, true)
	gesture := gestureFixture()
	gesture.SpeedPercent = speed
	fixtures["creative-v2"] = gesture
	stroke := strokeFixture()
	stroke.SpeedPercent = speed
	fixtures["creative"] = stroke
	mini := layeredSpeedFixture("short", speed)
	mini.MinPercent, mini.RangeFloorPercent, mini.RangeCeilingPercent = 90, 10, 10
	fixtures["minimum-width"] = mini
	defaultSpec := DefaultFlowSpec()
	defaultSpec.SpeedPercent = speed
	fixtures["default"] = defaultSpec
	// Deliberately provoke tiny field lobes while keeping ordinary authored
	// strokes. This distinguishes geometry cleanup from point decimation.
	chatter := layeredTransferFixture()
	chatter.SpeedPercent, chatter.LoopCycles = speed, 16
	chatter.RangeFloorPercent, chatter.RangeCeilingPercent = 10, 20
	chatter.Layers = []FlowLayer{{Axis: "center", AmountPercent: 100, PeriodCycles: 2, Shape: "wave"}}
	fixtures["field-lobes"] = chatter
	controls := freestyleControls()
	controls.PacePercent = 100
	stream := NewFreestyleFlow(0x5eed, speed, FreestyleSpec{StartStroke: 6, Strokes: 16, MinSpeedPercent: 1,
		Keyframes: []FreestyleKeyframe{{Controls: controls}}})
	fixtures["freestyle"] = stream
	return fixtures
}

type motionPathMetrics struct {
	ID                string  `json:"id"`
	PointsPerSecond   float64 `json:"points_per_second"`
	PeakWireVelocity  float64 `json:"peak_wire_velocity"`
	MaximumWireError  float64 `json:"maximum_wire_error"`
	RMSWireError      float64 `json:"rms_wire_error"`
	PositionTolerance float64 `json:"position_tolerance"`
	MissingTurns      int     `json:"missing_turns"`
	StationaryMillis  int64   `json:"stationary_ms"`
	TurnCount         int     `json:"turn_count"`
	LowSpeedShare     float64 `json:"low_speed_share"`
}

func measureMotionPath(entry Review, target MotionTarget, settings config.MotionSettings, policy string) motionPathMetrics {
	plan := NewMotionPlan("path-review", target, settings, 0, 0, time.Unix(0, 0))
	e := &Engine{plan: plan, settings: settings, preservePlanKnots: true, positionResolutionPercent: 1}
	configurePathReview(e, policy)
	metric := motionPathMetrics{ID: entry.ID, PositionTolerance: e.generatedPositionTolerance(false)}
	if len(entry.Wire) < 2 {
		return metric
	}
	factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
	end := min(plan.PeriodMillis, entry.Wire[len(entry.Wire)-1].TimeMillis)
	index, count, low := 1, 0, 0
	for at := entry.Wire[0].TimeMillis; at <= end; at++ {
		for index < len(entry.Wire)-1 && entry.Wire[index].TimeMillis < at {
			index++
		}
		left, right := entry.Wire[index-1], entry.Wire[index]
		if right.TimeMillis <= left.TimeMillis {
			continue
		}
		fraction := float64(at-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
		wire := left.PositionPercent + fraction*(right.PositionPercent-left.PositionPercent)
		position := plan.focus.apply(plan.curve.sampleFloat(float64(at) / factor))
		delta := math.Abs(wire - position)
		metric.MaximumWireError = math.Max(metric.MaximumWireError, delta)
		metric.RMSWireError += delta * delta
		velocity := math.Abs(plan.curve.velocityFloat(float64(at)/factor) * plan.focus.gain() * 1000 / factor)
		if velocity < entry.Summary.CommandedMeanTravelPerSecond*.1 {
			low++
		}
		count++
	}
	if count > 0 {
		metric.RMSWireError = math.Sqrt(metric.RMSWireError / float64(count))
		metric.LowSpeedShare = float64(low) / float64(count)
	}
	for i := 1; i < len(entry.Wire); i++ {
		left, right := entry.Wire[i-1], entry.Wire[i]
		if right.TimeMillis <= left.TimeMillis {
			continue
		}
		metric.PeakWireVelocity = math.Max(metric.PeakWireVelocity, math.Abs(right.PositionPercent-left.PositionPercent)*1000/float64(right.TimeMillis-left.TimeMillis))
		if left.PositionPercent == right.PositionPercent {
			metric.StationaryMillis += right.TimeMillis - left.TimeMillis
		}
	}
	duration := entry.Wire[len(entry.Wire)-1].TimeMillis - entry.Wire[0].TimeMillis
	metric.PointsPerSecond = float64(len(entry.Wire)-1) * 1000 / float64(duration)
	// Check actual reversals, not every easing polynomial knot. Rounded output
	// may shift a knot by at most one millisecond but cannot silently erase it.
	for _, turn := range perceptualReversals(plan.curve, plan.focus, plan.PeriodMillis) {
		if turn.at < entry.Wire[0].TimeMillis || turn.at > end {
			continue
		}
		metric.TurnCount++
		found := false
		for _, point := range entry.Wire {
			if math.Abs(float64(point.TimeMillis-turn.at)) <= 1 && math.Abs(point.PositionPercent-turn.position) <= .5001 {
				found = true
				break
			}
		}
		if !found {
			metric.MissingTurns++
		}
	}
	return metric
}
