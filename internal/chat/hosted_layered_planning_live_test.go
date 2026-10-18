//go:build liveeval

package chat

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type hostedPlanningCase struct {
	name, request string
	autopilot     bool
	requestAge    int
	start         func(*motion.FlowSpec)
	history       []llm.Message
	check         func(motion.FlowSpec, motion.FlowSpec) bool
}

func TestHostedLayeredLivePlanning(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 50
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	for _, tc := range hostedPlanningCases() {
		for repeat := range max(1, repeats) {
			t.Run(fmt.Sprintf("%s/%d", tc.name, repeat+1), func(t *testing.T) {
				before := DefaultLayeredScore(25)
				before.Seed = 841
				if tc.start != nil {
					tc.start(&before)
				}
				state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, Layered: &before, SpeedMinPercent: 10, SpeedMaxPercent: 50}
				start, calls := time.Now(), provider.calls
				var command *MotionCommand
				var err error
				nextCheck := 0
				if tc.autopilot {
					ago := max(5, tc.requestAge)
					state.UserRequests = []string{tc.request}
					state.UserRequestSecondsAgo = []int{ago}
					plannerCaps := caps
					plannerCaps.AutoTiming = true
					planner := AutopilotService{Provider: provider, Model: model, Capabilities: plannerCaps, MotionContext: &state, ReasoningMode: "off"}
					history := append([]llm.Message{}, tc.history...)
					history = append(history, llm.Message{Role: "user", Content: tc.request})
					context := AutopilotContext{Hosted: true, MotionMode: MotionModeLayered, CurrentFlow: &before, CurrentSpeed: before.SpeedPercent, SpeedMinPercent: 10, SpeedMaxPercent: 50, MotionChangeLevel: 4, AutoTiming: true, LastHumanSecondsAgo: &ago, SessionTracking: true, SessionSeconds: max(180, ago+60), SecondsAtCurrentSpeed: 120, RecentSpeeds: []SpeedStep{{SpeedPercent: 25, SecondsAgo: 120}, {SpeedPercent: 25, SecondsAgo: 60}, {SpeedPercent: 25, SecondsAgo: 20}}}
					var result AutopilotResponse
					result, err = planner.Complete(t.Context(), AutopilotKindMotion, Request{History: history, Message: AutopilotMotionMessage(context)})
					command, nextCheck = result.Motion, result.NextCheckSeconds
				} else {
					service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off"}
					var result Result
					result, err = service.Complete(t.Context(), Request{Message: tc.request}, nil)
					command = result.Response.Motion
				}
				after := before
				if command != nil && command.Layered != nil {
					after = *command.Layered
				}
				pass := err == nil && tc.check(before, after) && provider.calls-calls == 1
				if tc.autopilot {
					pass = pass && nextCheck >= config.AutopilotAutoTimingMinSeconds && nextCheck <= config.AutopilotAutoTimingMaxSeconds
				}
				row := hostedLayeredLiveRow(tc.name, model, tc.request, provider.raw, before, command, limits, start, err, pass)
				row["repeat"], row["provider_calls"], row["next_check_seconds"] = repeat+1, provider.calls-calls, nextCheck
				row["messages"] = provider.request.Messages
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				t.Logf("pass=%t speed=%d->%d elapsed_ms=%v err=%v", pass, before.SpeedPercent, after.SpeedPercent, row["elapsed_ms"], err)
				if !pass {
					t.Errorf("planning intent failed; retained raw proposal and compiled score")
				}
			})
		}
	}
}

func hostedPlanningCases() []hostedPlanningCase {
	pace := func(_, after motion.FlowSpec) bool { return after.SpeedPercent == 25 }
	cases := []hostedPlanningCase{
		{name: "fixed_pace", request: "Keep the base speed at 25 percent; gently vary the stroke location.", autopilot: true, check: pace},
		{name: "pace_paraphrase", request: "The current rate is exactly right. Let it stay there while you explore different reaches and locations.", autopilot: true, check: pace},
		{name: "pace_spanish", request: "Mantén la velocidad actual; cambia sólo la posición y la longitud de los movimientos.", autopilot: true, check: pace},
		{name: "lasting_pace", request: "Keep the base speed at 25 percent until I change my mind. You can develop the rest.", autopilot: true, requestAge: 600, check: pace},
		{name: "fixed_band", request: "Keep all travel between 20 and 70 percent throughout this session. You may develop pace and texture inside that band.", autopilot: true, start: func(s *motion.FlowSpec) { s.MinPercent, s.MaxPercent, s.RangeCeilingPercent = 20, 70, 50 }, check: func(_, a motion.FlowSpec) bool { return a.MinPercent == 20 && a.MaxPercent == 70 }},
		{name: "hold_all", request: "Keep this exact pattern and speed repeating until I tell you otherwise.", autopilot: true, check: func(b, a motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }},
		{name: "release_pace", request: "I'm changing that earlier instruction: raise the base speed to 35 percent now. Develop the location however you like.", autopilot: true, history: []llm.Message{{Role: "user", Content: "Keep the speed at 25 percent until I say otherwise."}, {Role: "assistant", Content: "The speed is staying at 25 percent."}}, check: func(_, a motion.FlowSpec) bool { return a.SpeedPercent == 35 }},
		{name: "custom_widths", request: "Alternate between both ends with widths from 20 to 40 percent, not the default short widths. Keep the current speed.", check: func(_, a motion.FlowSpec) bool {
			if a.SpeedPercent != 25 || a.RangeFloorPercent != 20 || a.RangeCeilingPercent != 40 {
				return false
			}
			for _, layer := range a.Layers {
				if layer.Axis == "center" && layer.Shape == "alternate" && layer.AmountPercent == 100 {
					return true
				}
			}
			return false
		}},
		{name: "custom_anchor", request: "Set the anchor to 72 percent and the stroke widths to 20 through 40 percent. Remove the center layer so the anchor stays fixed. Keep the speed and pace texture unchanged.", check: func(b, a motion.FlowSpec) bool {
			if a.SpeedPercent != b.SpeedPercent || a.PaceVariationPercent != b.PaceVariationPercent || a.AnchorPercent != 72 || a.RangeFloorPercent != 20 || a.RangeCeilingPercent != 40 {
				return false
			}
			for _, layer := range a.Layers {
				if layer.Axis == "center" {
					return false
				}
			}
			return pacePeriod(&a) == pacePeriod(&b)
		}},
	}
	for _, tc := range []struct {
		name, request string
		anchor        int
	}{
		{"base_anchor", "Anchor the varying strokes at the base of the current band. Keep the current pace.", 100},
		{"tip_anchor", "Anchor the varying strokes at the tip of the current band. Keep the current pace.", 0},
		{"centered", "Center all the strokes in the current band without roaming their location. Keep the current pace.", 0},
		{"alternate_ends", "Alternate short strokes between both ends of the current band. Keep the current pace.", 0},
		{"full_and_base", "Alternate strokes filling the band with short strokes anchored at its base. Keep the current pace.", 100},
		{"full_and_tip", "Alternate strokes filling the band with short strokes anchored at its tip. Keep the current pace.", 0},
		{"full_slider", "Make every stroke span the whole slider, from zero to one hundred. Keep the current pace.", 0},
		{"full_band", "Make every stroke fill the current band exactly. Keep its outer limits and the current pace.", 0},
		{"wander", "Replace the current periodic reach and location with irregular drifting reach and location. Keep the current pace.", 0},
	} {
		cases = append(cases, hostedPlanningCase{name: "geometry_" + tc.name, request: tc.request, start: func(s *motion.FlowSpec) {
			s.AnchorPercent = tc.anchor
			s.VariationMode = "waves"
			s.MinPercent, s.MaxPercent, s.RangeFloorPercent, s.RangeCeilingPercent = 10, 90, 15, 40
		}, check: func(b, a motion.FlowSpec) bool {
			if a.SpeedPercent != 25 || validateLayeredGeometry(a, tc.name) != nil {
				return false
			}
			if tc.name != "full_slider" && (a.MinPercent != b.MinPercent || a.MaxPercent != b.MaxPercent) {
				return false
			}
			if tc.name == "wander" {
				return a.VariationMode == "drift"
			}
			return true
		}})
	}
	return cases
}
