//go:build magichandy_labs

package main

// The Freestyle direction prototype is inert development tooling, not a new motion owner.
import (
	"context"
	"encoding/json"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"strings"
	"time"
)

func adaptive(ctx context.Context, p llm.Provider, model, message string, current motion.FlowSpec, limits config.MotionSettings, history []llm.Message) chat.LLMLabTrial {
	prompt := `Edit tendencies of an endless natural motion stream. Return only JSON with a brief reply and partial controls; controls:{} changes nothing. Preserve every unrequested control. Pace is 0..100 within the current saved speed band; length is 0..100 from short to full. Focus is 0=deep/base, 100=shallow/tip, placing the stroke within 0..100. Roaming lets its location wander away from the focus. Variety changes stroke length, pace and texture naturally, without a repeating loop. Accent is -100..100: negative makes travel toward the base faster, positive toward tip faster, zero is symmetric. Use only pace_percent, length_percent, focus_percent, roaming_percent, variety_percent and accent_percent. These are tendencies, not hard position bounds: variety can produce occasional full reaches. Never promise strict depth bounds this interface cannot enforce. No timed speed escalation. The host owns seeds, ramps and future stroke scheduling. Questions and exact holds require controls:{}.`
	props := map[string]any{}
	for _, k := range []string{"pace_percent", "length_percent", "focus_percent", "roaming_percent", "variety_percent", "accent_percent"} {
		minimum := 0
		if k == "accent_percent" {
			minimum = -100
		}
		props[k] = map[string]any{"type": "integer", "minimum": minimum, "maximum": 100}
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"reply": map[string]any{"type": "string"}, "controls": map[string]any{"type": "object", "properties": props, "required": []string{}, "additionalProperties": false}}, "required": []string{"reply", "controls"}, "additionalProperties": false})
	state, _ := json.Marshal(current.Freestyle.Keyframes[0].Controls)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: "Current controls: " + string(state) + "\nRequest: " + message})
	start := time.Now()
	raw, err := p.StreamChat(ctx, llm.ChatRequest{Model: model, Messages: messages, JSONSchema: schema}, nil)
	t := chat.LLMLabTrial{Message: message, Model: model, Method: "adaptive", Prompt: prompt, Before: current, After: current, Limits: limits, Raw: raw, ElapsedMillis: time.Since(start).Milliseconds(), ProviderCalls: 1, SchemaGuided: true, Changed: []string{}}
	if err != nil {
		t.Error = err.Error()
		return t
	}
	var response struct {
		Reply    string         `json:"reply"`
		Controls map[string]int `json:"controls"`
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&response); err != nil {
		t.Error = err.Error()
		return t
	}
	next := *motion.CloneFlowSpec(&current)
	encoded, _ := json.Marshal(next.Freestyle.Keyframes[0].Controls)
	fields := map[string]int{}
	_ = json.Unmarshal(encoded, &fields)
	for k, v := range response.Controls {
		if _, ok := props[k]; !ok {
			t.Error = "unknown control"
			return t
		}
		fields[k] = v
		t.Changed = append(t.Changed, k)
	}
	encoded, _ = json.Marshal(fields)
	_ = json.Unmarshal(encoded, &next.Freestyle.Keyframes[0].Controls)
	if err = next.Validate(limits); err != nil {
		t.Error = err.Error()
		return t
	}
	if strings.TrimSpace(response.Reply) == "" {
		t.Error = "empty reply"
		return t
	}
	t.After = next
	t.Reply = response.Reply
	t.Valid = true
	return t
}
