//go:build liveeval

package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Uses the actual generated Autopilot message, including rich state that used
// to hit the interactive 4 KiB limit. No transport or engine is constructed.
func TestHostedAutopilotDevelopsSession(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true, AutoTiming: true}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, request string
		counted, hold bool
	}{
		{"independent_variety", "Take the lead from here. Keep developing the pace, reach and pattern on your own; I won't send a new message for every change.", false, false},
		{"counted_variety", "Keep repeating two short strokes in the upper area followed by one full sweep. Develop the pace on your own while keeping that counted motif.", true, false},
		{"explicit_hold", "Keep this exact pattern and speed unchanged until I ask otherwise.", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt, state := richAutopilotInput()
			current := *motion.CloneFlowSpec(state.Layered)
			current.Layers = nil
			if !tc.counted {
				current.Phrase = nil
			}
			state.UserRequests = []string{tc.request}
			speeds := map[int]bool{current.SpeedPercent: true}
			shapes := map[string]bool{autopilotGeometry(current): true}
			calls := provider.calls
			for turn := 0; turn < 4; turn++ {
				before := *motion.CloneFlowSpec(&current)
				state.Layered = &before
				state.SpeedPercent = before.SpeedPercent
				state.UserRequestSecondsAgo = []int{70 + turn*35}
				prompt.CurrentFlow = &before
				prompt.CurrentSpeed = before.SpeedPercent
				prompt.SessionSeconds = 480 + turn*35
				prompt.SegmentIndex = turn + 4
				age := state.UserRequestSecondsAgo[0]
				prompt.LastHumanSecondsAgo = &age
				message := AutopilotMotionMessage(prompt)
				started := time.Now()
				service := AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state,
					MaxTokens: 2048, ReasoningMode: "off", Temperature: 0.81, PromptBudget: PromptBudgetSettings{ContextSize: config.MinimumExternalContext, MaxOutputTokens: 2048}}
				response, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: message, History: []llm.Message{{Role: "user", Content: tc.request}}})
				if response.Motion != nil && response.Motion.Layered != nil {
					current = *motion.CloneFlowSpec(response.Motion.Layered)
				}
				valid := err == nil
				if tc.hold {
					valid = valid && reflect.DeepEqual(before, current)
				}
				if tc.counted {
					valid = valid && current.Phrase != nil && len(current.Phrase.Strokes) == 2
					if valid {
						valid = current.Phrase.Strokes[0].Count == 2 && current.Phrase.Strokes[1].Count == 1
					}
				}
				row := hostedLayeredLiveRow(fmt.Sprintf("%s_%d", tc.name, turn+1), model, tc.request, provider.raw, before, response.Motion, limits, started, err, valid)
				row["generated_context_bytes"] = len(message)
				row["messages"] = provider.request.Messages
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				t.Logf("turn=%d bytes=%d speed=%d->%d pass=%t err=%v", turn+1, len(message), before.SpeedPercent, current.SpeedPercent, valid, err)
				if !valid {
					t.Errorf("invalid planning outcome; retained for visual review")
				}
				speeds[current.SpeedPercent] = true
				shapes[autopilotGeometry(current)] = true
				prompt.RecentSpeeds = append(prompt.RecentSpeeds, SpeedStep{SpeedPercent: current.SpeedPercent, SecondsAgo: 0})
			}
			if provider.calls-calls != 4 {
				t.Errorf("provider calls=%d, want all four planning turns", provider.calls-calls)
			}
			if !tc.hold && len(speeds) < 2 {
				t.Error("no autonomous pace development")
			}
			if !tc.hold && !tc.counted && len(shapes) < 2 {
				t.Error("no autonomous pattern development")
			}
		})
	}
}

func autopilotGeometry(score motion.FlowSpec) string {
	score.SpeedPercent, score.Seed, score.PaceVariationPercent = 0, 0, 0
	layers := []motion.FlowLayer{}
	for _, layer := range score.Layers {
		if layer.Axis != "pace" {
			layers = append(layers, layer)
		}
	}
	score.Layers = layers
	raw, _ := json.Marshal(score)
	return string(raw)
}
