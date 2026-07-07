// Package chat orchestrates local LLM turns into app-level semantic actions.
package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// MotionActionNone leaves motion unchanged.
	MotionActionNone = "none"
	// MotionActionStart starts motion through the motion engine.
	MotionActionStart = "start"
	// MotionActionTarget retargets already running motion through the motion engine.
	MotionActionTarget = "target"
	// MotionActionStop stops motion through the motion engine.
	MotionActionStop = "stop"
)

// AssistantResponse is the only model output shape accepted by MagicHandy.
type AssistantResponse struct {
	Reply  string         `json:"reply"`
	Motion *MotionCommand `json:"motion,omitempty"`
}

// MotionCommand is semantic motion intent, not a transport command.
type MotionCommand struct {
	Action         string `json:"action"`
	PatternID      string `json:"pattern_id,omitempty"`
	LibraryBlockID string `json:"library_block_id,omitempty"`
	SpeedPercent   *int   `json:"speed_percent,omitempty"`
}

// ParseAssistantResponse validates one strict JSON response from the model.
func ParseAssistantResponse(raw string) (AssistantResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AssistantResponse{}, errors.New("assistant response is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response AssistantResponse
	if err := decoder.Decode(&response); err != nil {
		return AssistantResponse{}, fmt.Errorf("assistant response must be strict JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return AssistantResponse{}, errors.New("assistant response must contain exactly one JSON object")
	}
	if err := validateAssistantResponse(&response); err != nil {
		return AssistantResponse{}, err
	}
	return response, nil
}

func validateAssistantResponse(response *AssistantResponse) error {
	response.Reply = strings.TrimSpace(response.Reply)
	if response.Reply == "" {
		return errors.New("assistant response reply is required")
	}
	if response.Motion == nil {
		return nil
	}

	response.Motion.Action = strings.ToLower(strings.TrimSpace(response.Motion.Action))
	response.Motion.PatternID = strings.ToLower(strings.TrimSpace(response.Motion.PatternID))
	switch response.Motion.Action {
	case MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionStop:
	default:
		return fmt.Errorf("unknown motion action %q", response.Motion.Action)
	}
	if response.Motion.PatternID != "" && !validPatternID(response.Motion.PatternID) {
		return fmt.Errorf("unknown motion pattern %q", response.Motion.PatternID)
	}
	if strings.TrimSpace(response.Motion.LibraryBlockID) != "" {
		return nil
	}
	if response.Motion.SpeedPercent != nil {
		speed := *response.Motion.SpeedPercent
		if speed < 1 || speed > 100 {
			return errors.New("motion speed_percent must be between 1 and 100")
		}
	}
	if response.Motion.Action == MotionActionNone && (response.Motion.PatternID != "" || response.Motion.SpeedPercent != nil) {
		return errors.New("motion action none cannot include target fields")
	}
	return nil
}

func validPatternID(patternID string) bool {
	switch patternID {
	case "stroke", "pulse", "tease":
		return true
	default:
		return false
	}
}
