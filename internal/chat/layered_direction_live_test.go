//go:build liveeval && magichandy_labs

package chat

import (
	"context"
	"encoding/json"
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

// Opt-in comparisons never dispatch motion. A candidate can add the compact
// direction block to an unchanged local contract without enabling it in the app.
type directionTrialProvider struct {
	llm.Provider
	request      llm.ChatRequest
	candidate    bool
	intermediate bool
	original     string
}

func (p *directionTrialProvider) StreamChat(ctx context.Context, r llm.ChatRequest, emit func(string) error) (string, error) {
	if p.candidate {
		var schema map[string]any
		if err := json.Unmarshal(r.JSONSchema, &schema); err != nil {
			return "", err
		}
		addDirectionTrialSchema(schema)
		r.JSONSchema, _ = json.Marshal(schema)
		r.Messages[0].Content += "\n" + localDirectionTrialGuide
		if p.intermediate {
			r.Messages[0].Content += " Put directional_pace in its own edits list item, just like the other groups."
		}
	}
	p.request = r
	raw, err := p.Provider.StreamChat(ctx, r, emit)
	p.original = raw
	if err == nil && p.candidate && p.intermediate {
		// Trial-only representation adapter: all model-authored values survive.
		// The production group allowlist is extended only after a successful trial.
		root, parseErr := uniqueLayeredObject(json.RawMessage(raw))
		var groups []map[string]json.RawMessage
		if parseErr == nil && json.Unmarshal(root["edits"], &groups) == nil {
			fields := map[string]json.RawMessage{}
			for _, group := range groups {
				for key, value := range group {
					if _, exists := fields[key]; exists {
						return raw, fmt.Errorf("duplicate trial group %s", key)
					}
					fields[key] = value
				}
			}
			root["edits"], _ = json.Marshal(fields)
			encoded, _ := json.Marshal(root)
			raw = string(encoded)
		}
	}
	return raw, err
}

func addDirectionTrialSchema(v any) {
	switch x := v.(type) {
	case map[string]any:
		if props, ok := x["properties"].(map[string]any); ok {
			if edits, ok := props["edits"].(map[string]any); ok {
				if fields, ok := edits["properties"].(map[string]any); ok {
					fields["directional_pace"] = hostedDirectionalPaceSchema()
				}
				if items, ok := edits["items"].(map[string]any); ok {
					if choices, ok := items["anyOf"].([]any); ok {
						items["anyOf"] = append(choices, map[string]any{"type": "object", "properties": map[string]any{"directional_pace": hostedDirectionalPaceSchema()}, "required": []string{"directional_pace"}, "additionalProperties": false})
					}
				}
			}
		}
		for _, child := range x {
			addDirectionTrialSchema(child)
		}
	case []any:
		for _, child := range x {
			addDirectionTrialSchema(child)
		}
	}
}

const localDirectionTrialGuide = `Use directional_pace:{up_rate_percent,down_rate_percent} inside edits. Both integers1..100 multiply base rate. Up=tip100; down=base0. Omit to keep; both100 restores equal timing. Preserve reach and other timing. This is travel direction, not time-varying pace.`

func TestLayeredDirectionalLive(t *testing.T) {
	level := os.Getenv("MAGICHANDY_DIRECTION_LEVEL")
	if level == "" {
		t.Skip("set a capability level and inert provider")
	}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: level == config.ModelCapabilitySimple, AdvancedMotion: level == config.ModelCapabilityAdvanced, IntermediateMotion: level == config.ModelCapabilityIntermediate}
	caps.DirectionalPace = os.Getenv("MAGICHANDY_DIRECTION_CANDIDATE") == "1"
	var provider llm.Provider
	model := os.Getenv("MAGICHANDY_EVAL_MODEL")
	if endpoint := os.Getenv("MAGICHANDY_EVAL_URL"); endpoint != "" {
		var err error
		provider, err = llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{BaseURL: endpoint, Model: model, ContextSize: config.MinimumLocalContext(level), Timeout: 5 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		provider, model = hostedLayeredLiveProvider(t)
		caps.PreserveConversationText = true
	}
	maxTokens := config.DefaultLLMMaxOutputTokens
	if caps.IntermediateMotion {
		maxTokens = 512
	}
	if caps.AdvancedMotion {
		maxTokens = 1024
	}
	p := &directionTrialProvider{Provider: provider, candidate: os.Getenv("MAGICHANDY_DIRECTION_CANDIDATE") == "1", intermediate: caps.IntermediateMotion}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	base := DefaultLayeredScore(60)
	base.MinPercent, base.MaxPercent, base.RangeFloorPercent, base.RangeCeilingPercent = 0, 100, 100, 100
	base.Layers, base.PaceVariationPercent = nil, 0
	asymmetric := *motion.CloneFlowSpec(&base)
	asymmetric.DirectionalPace = &motion.DirectionalPace{UpRatePercent: 30, DownRatePercent: 100}
	checks := []struct {
		name, message string
		before        motion.FlowSpec
		check         func(motion.FlowSpec) bool
	}{
		{"quick_down", "Slam down quickly and go back up slowly.", base, func(s motion.FlowSpec) bool {
			return directionContrast(s, false) && s.MinPercent == 0 && s.MaxPercent == 100
		}},
		{"quick_up", "Travel quickly toward the tip and slowly toward the base. Keep this range and the base speed.", base, func(s motion.FlowSpec) bool {
			return directionContrast(s, true) && sameDirectionGeometry(s, base) && s.SpeedPercent == base.SpeedPercent
		}},
		{"reverse", "Reverse the directional timing: make the upward leg quick and the downward leg slow. Keep the range and overall speed.", asymmetric, func(s motion.FlowSpec) bool {
			return directionContrast(s, true) && sameDirectionGeometry(s, base) && s.SpeedPercent == base.SpeedPercent
		}},
		{"equal", "Make both directions equally paced again, at the current base speed. Leave the reach alone.", asymmetric, func(s motion.FlowSpec) bool {
			return s.DirectionalPace == nil && sameDirectionGeometry(s, base) && s.SpeedPercent == base.SpeedPercent
		}},
		{"reach_only", "Keep the fast-down/slow-up timing and speed, but restrict all travel to 20 through 80.", asymmetric, func(s motion.FlowSpec) bool {
			return reflect.DeepEqual(s.DirectionalPace, asymmetric.DirectionalPace) && s.SpeedPercent == base.SpeedPercent && s.MinPercent == 20 && s.MaxPercent == 80
		}},
		{"speed_only", "Lower the base speed by exactly 10 points. Keep the reach and the fast-down/slow-up contrast unchanged.", asymmetric, func(s motion.FlowSpec) bool {
			return reflect.DeepEqual(s.DirectionalPace, asymmetric.DirectionalPace) && s.SpeedPercent == 50 && sameDirectionGeometry(s, base)
		}},
		{"exact", "Keep exactly this motion, including its directional timing. No changes.", asymmetric, func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, asymmetric) }},
		{"question", "Which direction is faster right now? Don't change the motion.", asymmetric, func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, asymmetric) }},
	}
	if caps.AdvancedMotion {
		checks = append(checks, struct {
			name, message string
			before        motion.FlowSpec
			check         func(motion.FlowSpec) bool
		}{"compound", "Repeat two short cycles between 70 and 100 then one full stroke from 0 to 100. Travel quickly downward and slowly upward on all of them. Keep the base speed.", base, func(s motion.FlowSpec) bool {
			m := &MotionCommand{Layered: &s}
			return directionContrast(s, false) && s.SpeedPercent == base.SpeedPercent && sessionPhraseMatches(m, []motion.PhraseStroke{{BottomPercent: 70, TopPercent: 100, Count: 2}, {BottomPercent: 0, TopPercent: 100, Count: 1}})
		}})
	}
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_SESSION_REPEATS"))
	repeats = max(1, min(repeats, 5))
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for round := range repeats {
		for _, tc := range checks {
			t.Run(fmt.Sprintf("%s/%d/%s", level, round+1, tc.name), func(t *testing.T) {
				state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &tc.before, SpeedMinPercent: 1, SpeedMaxPercent: 100}
				s := Service{Provider: p, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: maxTokens, SingleAttempt: true, PromptBudget: PromptBudgetSettings{ContextSize: config.MinimumLocalContext(level), MaxOutputTokens: maxTokens}}
				started := time.Now()
				result, err := s.Complete(t.Context(), Request{Message: tc.message}, nil)
				after := tc.before
				if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
					after = *result.Response.Motion.Layered
				}
				pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && tc.check(after)
				row := hostedLayeredLiveRow(fmt.Sprintf("%s-%d-%s", level, round+1, tc.name), model, tc.message, result.Raw, tc.before, result.Response.Motion, limits, started, err, pass)
				row["provider_request"], row["candidate"], row["capability_level"] = p.request, p.candidate, level
				row["original_raw"] = p.original
				rows = append(rows, row)
				t.Logf("pass=%t elapsed_ms=%v err=%v", pass, row["elapsed_ms"], err)
				if !pass {
					t.Error("direction intent or first-pass failure, retained in report")
				}
			})
		}
	}
}

func directionContrast(s motion.FlowSpec, upFast bool) bool {
	if s.DirectionalPace == nil {
		return false
	}
	fast, slow := s.DirectionalPace.DownRatePercent, s.DirectionalPace.UpRatePercent
	if upFast {
		fast, slow = slow, fast
	}
	return fast > slow && float64(fast)/float64(slow) >= 1.5
}

func sameDirectionGeometry(a, b motion.FlowSpec) bool {
	return a.MinPercent == b.MinPercent && a.MaxPercent == b.MaxPercent && a.RangeFloorPercent == b.RangeFloorPercent && a.RangeCeilingPercent == b.RangeCeilingPercent && reflect.DeepEqual(a.Phrase, b.Phrase)
}
