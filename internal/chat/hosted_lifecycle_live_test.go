//go:build liveeval

package chat

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type hostedLifecycleCase struct {
	name, message      string
	mode               MotionMode
	start, paused, off bool
	afterStop, counted bool
	holder             MotionHolder
}

// TestHostedStoppedMotionIntentLive calls the real selected model without any
// engine or transport. Every raw response survives in the capture, including
// false starts, missed starts and rejected contracts. No repair masks intent.
func TestHostedStoppedMotionIntentLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range hostedLifecycleCases() {
		if mode := os.Getenv("MAGICHANDY_LIFECYCLE_MODE"); mode != "" && mode != string(tc.mode) {
			continue
		}
		t.Run(string(tc.mode)+"/"+tc.name, func(t *testing.T) {
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
			before := DefaultLayeredScore(40)
			if tc.mode == MotionModeCreativeV2 {
				before = FreshCreativeV2Score(40)
			}
			before.Seed = 915
			state := MotionContext{MotionMode: tc.mode, Layered: &before, Paused: tc.paused, SpeedPercent: 40, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
			caps := Capabilities{Motion: !tc.off, MotionMode: tc.mode, Patterns: tc.mode == MotionModePattern, HostedMotion: true, PreserveConversationText: true, MotionHolder: tc.holder}
			svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, Patterns: hostedLifecyclePatterns(tc.mode), ReasoningMode: "off", MaxTokens: 512}
			request := Request{Message: tc.message}
			if tc.afterStop {
				request.History = []llm.Message{{Role: "user", Content: "Use a smooth flowing rhythm."}, {Role: "assistant", Content: "Playing a flowing rhythm."}, {Role: "user", Content: "Stop."}, {Role: "assistant", Content: "The device is stopped."}}
			}
			started, calls := time.Now(), provider.calls
			result, err := svc.Complete(t.Context(), request, nil)
			command := result.Response.Motion
			moves := command != nil && command.Action != MotionActionNone
			pass := err == nil && moves == tc.start && result.Response.Reply != "" && !result.Repaired && provider.calls-calls == 1
			if tc.start {
				pass = pass && command != nil && command.Action == MotionActionStart
			}
			if tc.counted && pass {
				pass = command.Layered != nil && command.Layered.Phrase != nil && len(command.Layered.Phrase.Strokes) == 2
				if pass {
					groups := command.Layered.Phrase.Strokes
					pass = groups[0].Count == 2 && groups[1].Count == 1 && groups[0].BottomPercent > groups[1].BottomPercent && groups[1].BottomPercent == 0 && groups[1].TopPercent == 100
				}
			}
			row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
			row["method"], row["motion_command"], row["reply"], row["provider_calls"] = tc.mode, command, result.Response.Reply, provider.calls-calls
			row["paused"], row["motion_enabled"], row["motion_holder"], row["history"] = tc.paused, !tc.off, tc.holder, request.History
			if command == nil || command.Layered == nil {
				delete(row, "after") // Never render an unrelated default Flow score.
			}
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("pass=%t calls=%d elapsed_ms=%v error=%v", pass, provider.calls-calls, row["elapsed_ms"], err)
			if !pass {
				t.Error("lifecycle intent failed; see retained model output")
			}
		})
	}
}

func hostedLifecyclePatterns(mode MotionMode) []PatternChoice {
	if mode != MotionModePattern {
		return nil
	}
	choices := []PatternChoice{}
	for _, definition := range motion.BuiltinPatternDefinitions() {
		if !slices.Contains(definition.Tags, motion.TagExperimental) && !slices.Contains(definition.Tags, motion.TagDeprecated) {
			choices = append(choices, PatternChoice{ID: string(definition.ID), Name: definition.Name, Description: definition.Description, Tags: definition.Tags, Weight: 1})
		}
	}
	return choices
}

func hostedLifecycleCases() []hostedLifecycleCase {
	cases := []hostedLifecycleCase{}
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2, MotionModeDynamic, MotionModePattern} {
		for _, tc := range []hostedLifecycleCase{
			{name: "fresh_motion", message: "Use syncopation.", start: true},
			{name: "after_stop", message: "Use syncopation.", start: true, afterStop: true},
			{name: "question", message: "What would syncopation feel like?"},
			{name: "future_settings", message: "Set up a syncopated pattern for later, don't move yet."},
			{name: "paused", message: "Use syncopation.", paused: true},
		} {
			tc.mode = mode
			cases = append(cases, tc)
		}
	}
	return append(cases,
		hostedLifecycleCase{name: "counted_pattern", mode: MotionModeLayered, message: "Repeat two short upper-end strokes followed by one full sweep from base to tip and back, smoothly, in that order.", start: true, counted: true},
		hostedLifecycleCase{name: "quoted_request", mode: MotionModeLayered, message: `Explain what "use syncopation" means.`, afterStop: true},
		hostedLifecycleCase{name: "after_stop_chat", mode: MotionModeLayered, message: "Thank you, that was helpful.", afterStop: true},
		hostedLifecycleCase{name: "llm_off", mode: MotionModeLayered, message: "Use syncopation.", off: true},
		hostedLifecycleCase{name: "script_owner", mode: MotionModeLayered, message: "Use syncopation.", off: true, holder: MotionHolderVideoScript},
	)
}
