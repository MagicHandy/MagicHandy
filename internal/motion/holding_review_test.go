//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// This opt-in exporter exercises the production fitter, retargeter and sampler
// without constructing a transport. Every trial, including failures, is kept.
func TestEdgeVisualReview(t *testing.T) {
	output := os.Getenv("MAGICHANDY_EDGE_REVIEW_OUTPUT")
	if output == "" {
		t.Skip("set inert Edge review output")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	var paths []map[string]any
	add := func(name string, target MotionTarget, limits config.MotionSettings, model, raw string) {
		entry := ReviewMotionOutput(target, limits)
		entry.ID = name
		entry.Name = name
		entry.Group = "edge"
		entry.Model = model
		entry.Raw = raw
		entries = append(entries, entry)
		if entry.Error != "" {
			t.Errorf("%s: %s", name, entry.Error)
		}
	}
	join := func(name string, from, to MotionTarget, limits config.MotionSettings) {
		old := NewMotionPlan("old", from, limits, 0, 0, time.Unix(0, 0))
		for _, fraction := range []float64{.17, .43, .71, .93} {
			at := int64(float64(old.PeriodMillis) * fraction)
			next := old.Retarget("hold", to, limits, at, time.Unix(0, 0))
			bridge := layeredTurnTransition(old, nil, &next, at)
			joined := bridge != nil
			if bridge == nil {
				bridge = newPlanTransition(old, nil, at)
			}
			var path [][3]float64
			var peakV, peakA, previousV float64
			var turns, tiny int
			lastDirection := 0
			lastTurn := old.SampleAt(at).PositionPercent
			for now := at - 250; now <= bridge.endMillis+1000; now += 5 {
				x := sampleMotionPath(next, bridge, now).PositionPercent
				v := (sampleMotionPath(next, bridge, now+1).PositionPercent - sampleMotionPath(next, bridge, now-1).PositionPercent) * 500
				if math.IsNaN(x) || x < -0.01 || x > 100.01 {
					t.Errorf("%s: out-of-range sample %f", name, x)
				}
				if now >= at && now <= bridge.endMillis {
					peakV = max(peakV, math.Abs(v))
					peakA = max(peakA, math.Abs(v-previousV)*200)
					dir := curveDirection(v)
					if dir != 0 && lastDirection != 0 && dir != lastDirection {
						if turns > 0 && math.Abs(x-lastTurn) < 1 {
							tiny++
						}
						turns++
						lastTurn = x
					}
					if dir != 0 {
						lastDirection = dir
					}
				}
				previousV = v
				path = append(path, [3]float64{float64(now-at) / 1000, x, v})
			}
			paths = append(paths, map[string]any{"name": fmt.Sprintf("%s/%.2f", name, fraction), "path": path, "spliced": joined, "tiny_turns": tiny, "peak_velocity": peakV, "peak_acceleration": peakA})
		}
	}
	for _, kind := range []string{"layered", "creative_v2", "library", "steps"} {
		for _, speed := range []int{15, 50, 90} {
			spec := DefaultFlowSpec()
			spec.SpeedPercent = speed
			spec.PaceVariationPercent = 0
			if kind == "creative_v2" {
				g := DefaultGestureSpec()
				spec.Gesture = &g
			}
			if kind == "steps" {
				spec.Steps = []FlowStep{{MinPercent: 5, MaxPercent: 95, SpeedPercent: speed, Cycles: 2}, {MinPercent: 55, MaxPercent: 95, SpeedPercent: speed, Cycles: 3}}
			}
			before, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "library" {
				before = MotionTarget{PatternID: "flow-full-sweeps", SpeedPercent: speed}
			}
			p := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
			summary := ReviewMotionOutput(before, settings).Summary
			hold, err := HoldingTarget(ActiveMotionState{Target: p.Target, Perceptual: summary}, settings)
			if err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("%s/%d", kind, speed)
			add(name+"/before", before, settings, "", "")
			add(name+"/hold", hold, settings, "", "")
			join(name+"/entry", before, hold, settings)
			join(name+"/return", hold, before, settings)
		}
	}
	for _, file := range strings.Split(os.Getenv("MAGICHANDY_EDGE_LIVE_REPORTS"), ",") {
		if file == "" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var report struct {
			Turns []struct {
				Model, Raw, Wording string
				Valid               bool
				Context             struct {
					Previous int `json:"previous_speed_percent"`
					Holding  int `json:"holding_speed_percent"`
					Min      int `json:"speed_min_percent"`
					Max      int `json:"resume_speed_max_percent"`
					Lo       int `json:"holding_range_min_percent"`
					Hi       int `json:"holding_range_max_percent"`
				}
				Plan struct {
					Speed int    `json:"resume_speed_percent"`
					Style string `json:"resume_style"`
				}
			}
		}
		if err = json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		for i, row := range report.Turns {
			name := fmt.Sprintf("%s/%s/%d", row.Model, row.Wording, i)
			if !row.Valid {
				entries = append(entries, Review{ID: name, Name: name, Group: "edge", Model: row.Model, Raw: row.Raw, Error: "rejected response"})
				continue
			}
			limits := settings
			limits.SpeedMinPercent, limits.SpeedMaxPercent = row.Context.Min, row.Context.Max
			spec := DefaultFlowSpec()
			spec.MinPercent, spec.MaxPercent = row.Context.Lo, row.Context.Hi
			spec.RangeFloorPercent, spec.RangeCeilingPercent = spec.MaxPercent-spec.MinPercent, spec.MaxPercent-spec.MinPercent
			spec.SpeedPercent = row.Context.Holding
			spec.PaceVariationPercent = 0
			spec.AnchorPercent = 50
			spec.TurnSoftnessPercent = 85
			hold, err := FlowTarget(spec, limits)
			if err != nil {
				t.Fatal(err)
			}
			if row.Plan.Style == "previous" {
				spec = DefaultFlowSpec()
				spec.PaceVariationPercent = 0
			}
			spec.SpeedPercent = row.Plan.Speed
			next, err := FlowTarget(spec, limits)
			if err != nil {
				t.Fatal(err)
			}
			add(name+"/holding", hold, limits, row.Model, row.Raw)
			add(name+"/return", next, limits, row.Model, row.Raw)
			join(name+"/return", hold, next, limits)
		}
	}
	for suffix, value := range map[string]any{"-atlas.json": map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries}, "-handoffs.json": paths} {
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output+suffix, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("rendered %d steady entries and %d handoffs", len(entries), len(paths))
}
