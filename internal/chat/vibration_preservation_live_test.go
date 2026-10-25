//go:build liveeval

package chat

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// A matched follow-up to the blinded quality review: measure unwanted edits,
// not merely whether the requested parameter appears in the response.
func TestVibrationFormatPreservationLive(t *testing.T) {
	variant := os.Getenv("MAGICHANDY_VIBRATION_FORMAT")
	if variant != "required" && variant != "grouped" {
		t.Skip("choose required or grouped format")
	}
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, message string
		edit          func(*motion.FlowSpec)
		vibration     *motion.VibrationSpec
	}{
		{"speed_only", "Change only the Handy base speed from 40 to 55. Preserve every other Handy setting and the vibration wave exactly.", func(f *motion.FlowSpec) { f.SpeedPercent = 55 }, nil},
		{"speed_and_period", "Change the Handy base speed to 55 and halve the vibrator wave's eight-second cycle. Keep all other settings on both devices exactly as they are.", func(f *motion.FlowSpec) { f.SpeedPercent = 55 }, &motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 4}},
		{"pace_variation", "Reduce only the Handy pace variation from 12 to 5 percent. Keep base speed, geometry, both layers and vibration unchanged.", func(f *motion.FlowSpec) { f.PaceVariationPercent = 5 }, nil},
		{"memory_only", "Change only the Handy motion memory from 10 to 14 cycles. Keep its exact geometry, base speed, pace variation, layers and all vibration settings.", func(f *motion.FlowSpec) { f.MemoryCycles = 14 }, nil},
		{"floor_and_peak", "Raise the Handy's minimum stroke width from 20 to 25 percent without changing anything else in its movement. Separately, reduce only the vibrator wave's peak from 70 to 55 percent.", func(f *motion.FlowSpec) { f.RangeFloorPercent = 25 }, &motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 55, PeriodSeconds: 8}},
		{"wave_only", "Keep the Handy completely unchanged. Make only the vibrator wave twice as slow, preserving both its low and peak levels.", nil, &motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 16}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := DefaultLayeredScore(40)
			before.Seed = 931
			want := before
			if tc.edit != nil {
				tc.edit(&want)
			}
			current := motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 8}
			state := MotionContext{MotionMode: MotionModeLayered, Layered: &before, Running: true, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80,
				Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true, Armed: true, Follow: "speed", Motors: 2, Current: &current}}}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
			wrapped := &vibrationFormatProvider{Provider: provider, variant: variant}
			svc := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 1000}
			started := time.Now()
			result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
			command := result.Response.Motion
			after := before
			var spec *motion.VibrationSpec
			if command != nil {
				spec = command.Vibration
				if command.Layered != nil {
					after = *command.Layered
				}
			}
			pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && reflect.DeepEqual(effectiveWidth(after), effectiveWidth(want)) && reflect.DeepEqual(spec, tc.vibration)
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
			row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
			row["variant"], row["wire_raw"], row["reply"], row["vibration"] = variant, wrapped.raw, result.Response.Reply, spec
			row["current_vibration"], row["provider_calls"], row["repair_raw"], row["expected_after"] = current, wrapped.calls, result.RepairRaw, want
			row["motion_unchanged"] = command == nil || command.Layered == nil
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("exact_preservation=%t elapsed_ms=%v error=%v", pass, row["elapsed_ms"], err)
			if !pass {
				t.Error("unwanted edits or requested change missing; retained for review")
			}
		})
	}
}

func effectiveWidth(f motion.FlowSpec) motion.FlowSpec {
	// Zero is the engine's implicit full-band ceiling; an explicit equivalent
	// value is not an unwanted pattern change.
	if f.RangeCeilingPercent == 0 {
		f.RangeCeilingPercent = f.MaxPercent - f.MinPercent
	}
	return f
}
