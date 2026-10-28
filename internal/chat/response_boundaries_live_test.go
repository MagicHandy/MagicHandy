//go:build liveeval

package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Generate real replies first, then replay equivalent encodings without another
// model call. This distinguishes parser tolerance from changes in model intent.
// No prompt/schema is relaxed, and no transport is created.
func TestResponseBoundariesLive(t *testing.T) {
	provider, model, advanced := naturalLiveProvider(t)
	if !advanced && !naturalIntermediate() {
		t.Skip("Intermediate or Advanced required")
	}
	limits := config.DefaultSettings().Motion
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, message string
		stopped       bool
		check         func(motion.FlowSpec, *MotionCommand) bool
	}{
		{"full_slider", "Use the entire device range on every stroke and set speed to 55 percent.", false, func(s motion.FlowSpec, _ *MotionCommand) bool {
			if s.RangeCeilingPercent == 0 {
				s.RangeCeilingPercent = s.MaxPercent - s.MinPercent
			}
			return s.MinPercent == 0 && s.MaxPercent == 100 && layeredFullGeometry(s) && s.SpeedPercent == 55
		}},
		{"tip_anchor", "Anchor each return at the tip, remove center wandering, and reduce speed to 35 percent. Keep the width variation.", false, func(s motion.FlowSpec, _ *MotionCommand) bool {
			return s.AnchorPercent == 100 && s.SpeedPercent == 35 && noCenterMovement(s)
		}},
		{"base_anchor", "Anchor each return at the base, remove center wandering, and increase speed to 55 percent. Keep the width variation.", false, func(s motion.FlowSpec, _ *MotionCommand) bool {
			return s.AnchorPercent == 0 && s.SpeedPercent == 55 && noCenterMovement(s)
		}},
		{"compound_range", "Use only 20 to 90 percent of travel, centered within that range, with strokes 15 to 25 percent wide and speed 35 percent.", false, func(s motion.FlowSpec, _ *MotionCommand) bool {
			return s.MinPercent == 20 && s.MaxPercent == 90 && s.RangeFloorPercent == 15 && s.RangeCeilingPercent == 25 && s.SpeedPercent == 35 && s.AnchorPercent == 50
		}},
		{"relative_speed", "Reduce the current speed by ten percentage points, leaving everything else unchanged.", false, func(s motion.FlowSpec, _ *MotionCommand) bool {
			want := boundaryScore()
			want.SpeedPercent = 35
			return reflect.DeepEqual(s, want)
		}},
		{"continue", "Continue where we left off, with the same motion settings.", true, func(s motion.FlowSpec, m *MotionCommand) bool {
			return m != nil && m.Action == MotionActionStart && reflect.DeepEqual(s, boundaryScore())
		}},
		{"question_stopped", "What speed is saved? Just explain; leave playback stopped.", true, func(s motion.FlowSpec, m *MotionCommand) bool {
			return m == nil && reflect.DeepEqual(s, boundaryScore())
		}},
		{"question_running", "What does the pace layer do? Explain only; do not change playback.", false, func(s motion.FlowSpec, m *MotionCommand) bool {
			return m == nil && reflect.DeepEqual(s, boundaryScore())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := boundaryScore()
			state := naturalState(&before, MotionModeLayered, limits)
			state.Running, state.Outputs = !tc.stopped, &OutputContext{Strokers: 1}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced, IntermediateMotion: !advanced, PreserveConversationText: advanced}
			svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget(), SingleAttempt: true, ReasoningMode: "off"}
			start, calls := time.Now(), provider.calls
			result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
			after := before
			if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
				after = *result.Response.Motion.Layered
			}
			pass := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && provider.calls-calls == 1 && tc.check(after, result.Response.Motion)
			row := map[string]any{"case": tc.name, "model": model, "message": tc.message, "before": before, "limits": limits, "state": state, "caps": caps, "raw": result.Raw, "raw_attempts": provider.raws[calls:], "command": result.Response.Motion, "pass": pass, "error": fmt.Sprint(err), "elapsed_ms": time.Since(start).Milliseconds()}
			variants := []map[string]any{}
			if err == nil {
				for name, raw := range boundaryRepresentations(result.Raw, advanced) {
					response, parseErr := svc.parseLayeredResponse(raw, before, state, limits, false)
					variants = append(variants, map[string]any{"name": name, "raw": raw, "accepted": parseErr == nil, "equivalent": parseErr == nil && reflect.DeepEqual(response.Motion, result.Response.Motion), "error": fmt.Sprint(parseErr)})
				}
			}
			row["variants"] = variants
			rows = append(rows, row)
			t.Logf("intent=%t elapsed_ms=%d variants=%d", pass, row["elapsed_ms"], len(variants))
			if !pass {
				t.Error("generation failed intent; retained in capture")
			}
		})
	}
}

func boundaryScore() motion.FlowSpec {
	score := DefaultLayeredScore(45)
	score.Seed = 20261028
	return score
}

func noCenterMovement(score motion.FlowSpec) bool {
	for _, layer := range score.Layers {
		if layer.Axis == "center" && layer.AmountPercent != 0 {
			return false
		}
	}
	return true
}

func boundaryRepresentations(raw string, advanced bool) map[string]string {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return nil
	}
	variants := map[string]string{}
	root["vibration"] = json.RawMessage("null")
	encoded, _ := json.Marshal(root)
	variants["unavailable_null_vibration"] = string(encoded)
	delete(root, "vibration")
	if !advanced {
		var groups []map[string]json.RawMessage
		if json.Unmarshal(root["edits"], &groups) != nil || len(groups) < 2 {
			return variants
		}
		joined := map[string]json.RawMessage{}
		for _, group := range groups {
			for key, value := range group {
				if joined[key] != nil {
					return variants
				}
				joined[key] = value
			}
		}
		root["edits"], _ = json.Marshal([]map[string]json.RawMessage{joined})
		encoded, _ = json.Marshal(root)
		variants["joined_independent_groups"] = string(encoded)
		return variants
	}
	edits, err := uniqueLayeredObject(root["edits"])
	if err != nil {
		return variants
	}
	var geometry, composition string
	_ = json.Unmarshal(edits["geometry"], &geometry)
	_ = json.Unmarshal(edits["composition"], &composition)
	if composition == "full_slider" {
		geometry = "full_slider"
	}
	controls := map[string]json.RawMessage{}
	if edits["controls"] != nil {
		if json.Unmarshal(edits["controls"], &controls) != nil {
			return variants
		}
	}
	switch geometry {
	case "full_slider":
		controls["min_percent"] = json.RawMessage(`{"op":"set","value":0}`)
		controls["max_percent"] = json.RawMessage(`{"op":"set","value":100}`)
	case "tip_anchor", "base_anchor", "centered":
		anchor := 50
		if geometry == "tip_anchor" {
			anchor = 100
		}
		if geometry == "base_anchor" {
			anchor = 0
		}
		controls["anchor_percent"] = json.RawMessage(fmt.Sprintf(`{"op":"set","value":%d}`, anchor))
	default:
		return variants
	}
	edits["controls"], _ = json.Marshal(controls)
	root["edits"], _ = json.Marshal(edits)
	encoded, _ = json.Marshal(root)
	variants["equivalent_geometry_controls"] = string(encoded)
	return variants
}

func TestResponseBoundaryReplay(t *testing.T) {
	path := os.Getenv("MAGICHANDY_BOUNDARY_REPLAY")
	if path == "" {
		t.Skip("set captured boundary report")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Turns []struct {
			Case     string
			Before   motion.FlowSpec
			Limits   config.MotionSettings
			State    MotionContext
			Caps     Capabilities
			Raw      string
			Command  *MotionCommand
			Pass     bool
			Variants []struct {
				Name, Raw string
				Accepted  bool
			}
		}
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, turn := range capture.Turns {
		svc := Service{Capabilities: &turn.Caps}
		for _, variant := range turn.Variants {
			response, err := svc.parseLayeredResponse(variant.Raw, turn.Before, turn.State, turn.Limits, false)
			pass := err == nil && reflect.DeepEqual(response.Motion, turn.Command)
			rows = append(rows, map[string]any{"case": turn.Case + "/" + variant.Name, "before": turn.Before, "limits": turn.Limits, "command": response.Motion, "raw_attempts": []string{variant.Raw}, "baseline_accepted": variant.Accepted, "pass": pass, "original_intent_pass": turn.Pass, "error": fmt.Sprint(err)})
			if !pass {
				t.Errorf("%s/%s changed semantics or failed: %v", turn.Case, variant.Name, err)
			}
		}
		// The exact original response must not change meaning either.
		if turn.Command != nil {
			response, err := svc.parseLayeredResponse(turn.Raw, turn.Before, turn.State, turn.Limits, false)
			if err != nil || !reflect.DeepEqual(response.Motion, turn.Command) {
				t.Errorf("original %s regressed: %v", turn.Case, err)
			}
		}
	}
}
