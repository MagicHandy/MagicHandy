//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The three wire formats have identical engine capabilities. Only the
// structural adapter below differs; it never interprets the user's language.
func TestVibrationFormatQualityLive(t *testing.T) {
	variant := os.Getenv("MAGICHANDY_VIBRATION_FORMAT")
	if variant != "optional" && variant != "required" && variant != "grouped" {
		t.Skip("choose a vibration wire-format experiment")
	}
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, message, want string
		linear              bool
	}{
		{"rolling_swell", "Give the vibrator a slow rolling swell, with a clearly gentle low and a stronger peak. Keep the Handy moving exactly as it is.", "wave", false},
		{"distinct_pulses", "Make vibration give distinct short pulses about once a second, with silence between them and rounded changes. Leave the Handy unchanged.", "pulse", false},
		{"relative_cycle", "Keep the vibrator's 15-to-70 percent wave but make its current eight-second cycle twice as fast. Preserve the Handy exactly.", "wave4", false},
		{"smaller_peak", "Change only the vibrator wave's peak from 70 to 55 percent. Preserve its 15 percent minimum, eight-second period and all Handy motion.", "peak55", false},
		{"compound_phrase", "On The Handy, repeat two short strokes at the tip followed by one full-device sweep, with natural joins. Independently, let the vibrator swell slowly between gentle and strong levels.", "wave", true},
		{"preserve_vibration", "Leave the vibration wave unchanged. On the Handy alternate short strokes between both ends, with a faster pace at the upper end. Keep the base speed at 40.", "none", true},
		{"quiet_then_return", "Set vibration to a gentle steady 20 percent of my allowed intensity. Keep the Handy's current movement unchanged.", "steady20", false},
		{"creative_choice", "Choose a fresh vibration rhythm with noticeable contrast and smooth changes. Keep the Handy's movement exactly as it is. Describe the rhythm you choose.", "vary", false},
		{"shared_compound", "Make The Handy use full-device sweeps at a steady 55 percent base speed, while the vibrator gives a separate three-second pulse from 10 to 60 percent of my allowed intensity.", "pulse3", true},
		{"future_only", "Later I may want strong bursts from the vibrator and shorter Handy strokes. For now explain the options and preserve both outputs.", "none", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := DefaultLayeredScore(40)
			before.Seed = 931
			current := motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 8}
			state := MotionContext{MotionMode: MotionModeLayered, Layered: &before, Running: true, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80,
				Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true, Armed: true, Follow: "speed", Motors: 2, Current: &current}}}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
			wrapped := &vibrationFormatProvider{Provider: provider, variant: variant}
			svc := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 1000}
			started := time.Now()
			result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
			command := result.Response.Motion
			var spec *motion.VibrationSpec
			if command != nil {
				spec = command.Vibration
			}
			pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != ""
			if tc.linear {
				pass = pass && command != nil && command.Layered != nil
			} else {
				pass = pass && (command == nil || command.Action == MotionActionNone)
			}
			switch tc.want {
			case "none":
				pass = pass && spec == nil
			case "wave":
				pass = pass && spec != nil && spec.Mode == "wave" && spec.MinPercent < spec.MaxPercent && spec.PeriodSeconds >= 4
			case "pulse":
				pass = pass && spec != nil && spec.Mode == "pulse" && spec.MinPercent == 0 && spec.PeriodSeconds >= 0.75 && spec.PeriodSeconds <= 1.5
			case "wave4":
				pass = pass && spec != nil && spec.Mode == "wave" && spec.MinPercent == 15 && spec.MaxPercent == 70 && spec.PeriodSeconds == 4
			case "peak55":
				pass = pass && spec != nil && spec.Mode == "wave" && spec.MinPercent == 15 && spec.MaxPercent == 55 && spec.PeriodSeconds == 8
			case "steady20":
				pass = pass && spec != nil && spec.Mode == "steady" && spec.MinPercent == 20 && spec.MaxPercent == 20
			case "vary":
				pass = pass && spec != nil && (spec.Mode == "wave" || spec.Mode == "pulse") && spec.MinPercent < spec.MaxPercent
			case "pulse3":
				pass = pass && spec != nil && spec.Mode == "pulse" && spec.MinPercent == 10 && spec.MaxPercent == 60 && spec.PeriodSeconds == 3
			}
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
			row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
			row["variant"], row["wire_raw"], row["reply"], row["vibration"] = variant, wrapped.raw, result.Response.Reply, spec
			row["current_vibration"], row["provider_calls"], row["repair_raw"] = current, wrapped.calls, result.RepairRaw
			row["motion_unchanged"] = command == nil || command.Layered == nil
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("intent=%t elapsed_ms=%v error=%v", pass, row["elapsed_ms"], err)
			if !pass {
				t.Error("pattern intent mismatch; retain for compiled review")
			}
		})
	}
}

type vibrationFormatProvider struct {
	llm.Provider
	variant, raw string
	calls        int
}

func (p *vibrationFormatProvider) StreamChat(ctx context.Context, request llm.ChatRequest, _ func(string) error) (string, error) {
	p.calls++
	if path := os.Getenv("MAGICHANDY_VIBRATION_GUIDE_FILE"); path != "" {
		guide, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		request.Messages = append([]llm.Message(nil), request.Messages...)
		for i := range request.Messages {
			if request.Messages[i].Role == "system" {
				request.Messages[i].Content = strings.ReplaceAll(request.Messages[i].Content, independentVibrationGuide, string(guide))
			}
		}
	}
	original := append(json.RawMessage(nil), request.JSONSchema...)
	if p.variant != "required" {
		var schema map[string]any
		if err := json.Unmarshal(request.JSONSchema, &schema); err != nil {
			return "", err
		}
		branches, _ := schema["oneOf"].([]any)
		for _, item := range branches {
			branch := item.(map[string]any)
			fields := branch["properties"].(map[string]any)
			required := branch["required"].([]any)
			kept := []any{}
			for _, key := range required {
				if key != "vibration" && !(p.variant == "grouped" && (key == "action" || key == "edits")) {
					kept = append(kept, key)
				}
			}
			if p.variant == "grouped" {
				fields["linear"] = map[string]any{"type": "object", "properties": map[string]any{"action": fields["action"], "edits": fields["edits"]}, "required": []string{"action", "edits"}, "additionalProperties": false}
				delete(fields, "action")
				delete(fields, "edits")
				kept = append(kept, "linear", "vibration")
			}
			branch["required"] = kept
		}
		request.JSONSchema, _ = json.Marshal(schema)
		if p.variant == "grouped" {
			request.Messages = append([]llm.Message(nil), request.Messages...)
			request.Messages[0].Content += `\nWIRE FORMAT: Put the linear action and edits together inside top-level linear. Keep vibration and reply outside it. Earlier action/edits guidance applies to linear.action/linear.edits. Always include vibration, using null to preserve it. Example: {"linear":{"action":"none","edits":{}},"vibration":{"mode":"wave","min_percent":10,"max_percent":50,"period_seconds":5},"reply":"A separate vibration swell; the linear motion stays unchanged."}`
		}
	}
	raw, err := p.Provider.StreamChat(ctx, request, nil)
	p.raw = raw
	if err != nil || p.variant != "grouped" {
		return raw, err
	}
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, err
	}
	linear, err := uniqueLayeredObject(root["linear"])
	if err != nil {
		return raw, err
	}
	delete(root, "linear")
	for key, value := range linear {
		if _, duplicate := root[key]; duplicate {
			return raw, fmt.Errorf("duplicate grouped field %s", key)
		}
		root[key] = value
	}
	encoded, err := json.Marshal(map[string]any{"proposal": root})
	if err != nil {
		return raw, err
	}
	return llm.DomainOutput(string(encoded), original)
}
