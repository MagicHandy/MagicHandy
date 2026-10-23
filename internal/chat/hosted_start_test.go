package chat

import (
	"fmt"
	"strings"
	"testing"
)

func hostedStartFixture(mode MotionMode, action string, speed int) string {
	if action == MotionActionNone {
		return `{"reply":"An explanation only.","motion":{"action":"none"}}`
	}
	geometry := ""
	if mode == MotionModeDynamic {
		geometry = `,"center_percent":50,"span_percent":80,"variation_percent":30,"segment_seconds":20`
	}
	return fmt.Sprintf(`{"reply":"Playing the requested rhythm.","motion":{"action":%q,"speed_percent":%d%s}}`, action, speed, geometry)
}

func TestHostedStartUsesModelIntentAndAuthoritativeState(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeDynamic, MotionModePattern} {
		for _, tc := range []struct {
			name, message, action, owner string
			hosted, disabled, paused     bool
			moves, reject                bool
			speed                        int
		}{
			{name: "terse motion", message: "Use syncopation.", hosted: true, moves: true},
			{name: "translated motion", message: "Usa un ritmo sincopado.", hosted: true, moves: true},
			{name: "model declines a question", message: "Would syncopation suit this?", action: MotionActionNone, hosted: true},
			{name: "model keeps it prospective", message: "Plan this for later, don't move.", action: MotionActionNone, hosted: true},
			{name: "local legacy unchanged", message: "Use syncopation."},
			{name: "paused", message: "Start it now.", hosted: true, paused: true, reject: true},
			{name: "off", message: "Start it now.", hosted: true, disabled: true},
			{name: "script", message: "Start it now.", hosted: true, disabled: true, owner: string(MotionHolderVideoScript)},
			{name: "saved speed limit", message: "Use syncopation.", hosted: true, speed: 90, reject: true},
		} {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				action, speed := tc.action, tc.speed
				if action == "" {
					action = MotionActionStart
				}
				if speed == 0 {
					speed = 30
				}
				raw := hostedStartFixture(mode, action, speed)
				provider := &scriptedProvider{responses: []string{raw}}
				caps := Capabilities{Motion: !tc.disabled, MotionMode: mode, HostedMotion: tc.hosted, PreserveConversationText: tc.hosted, MotionHolder: MotionHolder(tc.owner)}
				state := MotionContext{MotionMode: mode, Paused: tc.paused, SpeedMinPercent: 10, SpeedMaxPercent: 50}
				svc := Service{Provider: provider, Capabilities: &caps, MotionContext: &state, SingleAttempt: true}
				result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
				if (err != nil) != tc.reject || (result.Response.Motion != nil && result.Response.Motion.Action == MotionActionStart) != tc.moves || len(provider.requests) != 1 {
					t.Fatalf("start authority changed: %+v, %v; calls=%d", result, err, len(provider.requests))
				}
			})
		}
	}
}

func TestHostedStartGuidanceDoesNotChangeLocalLegacyPrompts(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeDynamic, MotionModePattern} {
		caps := Capabilities{Motion: true, MotionMode: mode}
		want := contractBase
		if mode == MotionModeDynamic {
			want = contractDynamic
		}
		if contractInstructions(caps) != want {
			t.Fatal("local legacy contract changed")
		}
		caps.HostedMotion = true
		if !strings.Contains(contractInstructions(caps), hostedStartGuide) || strings.Contains(contractInstructions(caps), "only for an explicit request to begin motion") {
			t.Fatal("hosted contract contradicts model-owned start intent")
		}
	}
}
