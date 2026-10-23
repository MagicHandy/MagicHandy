package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// EdgeContext is technical state for a button action. It deliberately carries
// no conversation, persona or history and adds no work to ordinary chat turns.
type EdgeContext struct {
	PreviousSpeed int `json:"previous_speed_percent"`
	HoldingSpeed  int `json:"holding_speed_percent"`
	SpeedMin      int `json:"speed_min_percent"`
	SpeedMax      int `json:"resume_speed_max_percent"`
	RangeMin      int `json:"holding_range_min_percent"`
	RangeMax      int `json:"holding_range_max_percent"`
}

// EdgePlan is the bounded result of one holding-plan request.
type EdgePlan struct {
	HoldSeconds int    `json:"hold_seconds"`
	ResumeSpeed int    `json:"resume_speed_percent"`
	ResumeStyle string `json:"resume_style"`
}

// EdgeService uses a compact technical contract only after Edge is pressed.
type EdgeService struct {
	Provider      llm.Provider
	Model         string
	ReasoningMode string
}

const edgePurpose = "The person pressed Edge to request a slow, gentle holding interval as a counterpoint to the current motion."

func edgeMessages(state EdgeContext, purpose string) []llm.Message {
	encoded, _ := json.Marshal(state)
	return []llm.Message{
		{Role: "system", Content: `You plan mechanical motion for an adult device. A gentle holding pattern is already running; do not issue device commands or dialogue. Choose how long to keep it and how Autopilot should return. Respect the explicit request for relief: allow time to settle and use a measured return, not a sudden escalation. The saved minimum remains a hard bound. Return JSON only: {"hold_seconds":integer 8..180,"resume_speed_percent":integer within speed_min_percent..resume_speed_max_percent,"resume_style":"previous" or "gentle"}. "previous" returns to the prior geometry at your chosen pace; "gentle" keeps the soft holding geometry at your chosen pace for the next stretch. Autopilot reconsiders after that stretch. Choose for this state; no fixed favourite duration or pace is required.`},
		{Role: "user", Content: purpose + "\nCurrent mechanical state: " + string(encoded)},
	}
}

// Complete makes one request; refusal or invalid output never triggers a retry.
func (s EdgeService) Complete(ctx context.Context, state EdgeContext) (EdgePlan, string, error) {
	return s.complete(ctx, state, edgePurpose)
}

func (s EdgeService) complete(ctx context.Context, state EdgeContext, purpose string) (EdgePlan, string, error) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"hold_seconds", "resume_speed_percent", "resume_style"},
		"properties": map[string]any{
			"hold_seconds":         map[string]any{"type": "integer", "minimum": 8, "maximum": 180},
			"resume_speed_percent": map[string]any{"type": "integer", "minimum": state.SpeedMin, "maximum": state.SpeedMax},
			"resume_style":         map[string]any{"type": "string", "enum": []string{"previous", "gentle"}},
		},
	})
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{Model: s.Model, Messages: edgeMessages(state, purpose), JSONSchema: schema, MaxTokens: 192, ReasoningMode: s.ReasoningMode, Temperature: 0.6}, nil)
	if err != nil {
		return EdgePlan{}, raw, err
	}
	var plan EdgePlan
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return EdgePlan{}, raw, fmt.Errorf("invalid holding plan: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return EdgePlan{}, raw, errors.New("holding plan must contain exactly one JSON object")
	}
	if plan.HoldSeconds < 8 || plan.HoldSeconds > 180 || plan.ResumeSpeed < state.SpeedMin || plan.ResumeSpeed > state.SpeedMax || (plan.ResumeStyle != "previous" && plan.ResumeStyle != "gentle") {
		return EdgePlan{}, raw, errors.New("holding plan exceeds the requested bounds")
	}
	return plan, raw, nil
}
