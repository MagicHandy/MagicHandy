//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type simplePlanTrialProvider struct {
	llm.Provider
	wire string
}

func withSimplePlanTrial(provider llm.Provider) llm.Provider {
	if strings.HasPrefix(os.Getenv("MAGICHANDY_SIMPLE_VARIANT"), "basis") {
		return &simplePlanTrialProvider{Provider: provider}
	}
	return provider
}
func (p *simplePlanTrialProvider) StreamChat(ctx context.Context, req llm.ChatRequest, delta func(string) error) (string, error) {
	var schema map[string]any
	if json.Unmarshal(req.JSONSchema, &schema) != nil {
		return p.Provider.StreamChat(ctx, req, delta)
	}
	var add func(map[string]any)
	add = func(s map[string]any) {
		for _, key := range []string{"oneOf", "anyOf"} {
			if branches, ok := s[key].([]any); ok {
				for _, b := range branches {
					add(b.(map[string]any))
				}
				return
			}
		}
		if props, ok := s["properties"].(map[string]any); ok && props["edits"] != nil {
			props["basis"] = map[string]any{"type": "string", "maxLength": 240}
			required, _ := s["required"].([]any)
			s["required"] = append(required, "basis")
		}
	}
	add(schema)
	req.JSONSchema, _ = json.Marshal(schema)
	req.Messages = append([]llm.Message{}, req.Messages...)
	req.Messages[0].Content += "\n\nBefore edits, write basis: one short planning sentence naming the human constraints that still apply, any restriction released or expired, and what may change now. Then emit only edits consistent with that basis. The basis is internal planning; reply remains the normal chat response. No fixed speed, location or variation is preferred."
	if strings.HasPrefix(os.Getenv("MAGICHANDY_SIMPLE_VARIANT"), "basis_") {
		for _, line := range strings.Split(req.Messages[0].Content, "\n") {
			var state map[string]any
			if json.Unmarshal([]byte(line), &state) == nil && state["autopilot"] != nil && state["recent_user_requests_oldest_first"] != nil {
				recent, _ := json.Marshal(state["recent_user_requests_oldest_first"])
				req.Messages[len(req.Messages)-1].Content += "\n\nHuman direction (oldest first, age in seconds): " + string(recent) + "\nJudge which wishes are still active before proposing edits. An ongoing exact hold means empty edits, including no evolution. A fixed speed leaves other dimensions free. Release restrictions when the human releases them or their stated duration ends."
			}
		}
	}
	if os.Getenv("MAGICHANDY_SIMPLE_VARIANT") == "basis_math" {
		req.Messages[len(req.Messages)-1].Content += "\nCheck the proposed numbers: a stroke width is a distance, at most outer max minus outer min, not the upper endpoint. Widen endpoints as well if you want a larger width. Preserve a requested fixed base speed exactly; variation in other dimensions is still allowed. A request to repeat exactly forbids reseeding too."
	}
	raw, err := p.Provider.StreamChat(ctx, req, delta)
	p.wire = raw
	if err != nil {
		return raw, err
	}
	root, e := uniqueLayeredObject(json.RawMessage(raw))
	if e != nil {
		return raw, e
	}
	delete(root, "basis")
	data, e := json.Marshal(root)
	return string(data), e
}

func naturalTrialWire(provider *outputLiveProvider) string {
	if trial, ok := provider.Provider.(*simplePlanTrialProvider); ok {
		return trial.wire
	}
	return provider.raw
}
