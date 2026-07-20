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
	Action       string `json:"action"`
	PatternID    string `json:"pattern_id,omitempty"`
	Intensity    *int   `json:"intensity,omitempty"`
	SpeedPercent *int   `json:"speed_percent,omitempty"`
	// Area optionally focuses motion on a named zone. Named zones localize to
	// bounded relative windows in deterministic code (the STGPT-RV area-focus
	// lesson: zones, never raw model-authored depth numbers).
	Area string `json:"area,omitempty"`
}

// Named area-focus zones the model may request. "full" explicitly clears an
// active focus.
const (
	AreaZoneTip   = "tip"
	AreaZoneShaft = "shaft"
	AreaZoneBase  = "base"
	AreaZoneFull  = "full"
)

// AreaZones lists the accepted area values in prompt order.
func AreaZones() []string {
	return []string{AreaZoneTip, AreaZoneShaft, AreaZoneBase, AreaZoneFull}
}

// PatternChoice is one enabled library entry exposed to the model as data.
type PatternChoice struct {
	ID          string
	Name        string
	Description string
	Tags        []string
	Weight      float64
}

// ParseAssistantResponse validates one strict JSON response from the model.
func ParseAssistantResponse(raw string) (AssistantResponse, error) {
	return parseAssistantResponse(raw, defaultPatternChoices(), false, nil)
}

// ParseAssistantResponseWithPatterns accepts only the supplied enabled IDs.
func ParseAssistantResponseWithPatterns(raw string, patterns []PatternChoice) (AssistantResponse, error) {
	return parseAssistantResponse(raw, patterns, true, nil)
}

func parseAssistantResponseForCapabilities(raw string, patterns []PatternChoice, capabilities Capabilities, context *MotionContext) (AssistantResponse, error) {
	response, err := decodeAssistantResponse(raw)
	if err != nil {
		return AssistantResponse{}, err
	}
	enforceCapabilities(&response, capabilities)
	patternsEnabled := capabilities.Motion && capabilities.Patterns
	var currentSpeed *int
	if patternsEnabled && context != nil && context.Running && context.SpeedPercent >= 1 && context.SpeedPercent <= 100 {
		speed := context.SpeedPercent
		currentSpeed = &speed
	}
	preserveCurrentPatternSpeed(&response, currentSpeed)
	if err := validateAssistantResponse(&response, patterns, patternsEnabled); err != nil {
		return AssistantResponse{}, err
	}
	return response, nil
}

func parseAssistantResponse(raw string, patterns []PatternChoice, curation bool, currentSpeed *int) (AssistantResponse, error) {
	response, err := decodeAssistantResponse(raw)
	if err != nil {
		return AssistantResponse{}, err
	}
	preserveCurrentPatternSpeed(&response, currentSpeed)
	if err := validateAssistantResponse(&response, patterns, curation); err != nil {
		return AssistantResponse{}, err
	}
	return response, nil
}

func decodeAssistantResponse(raw string) (AssistantResponse, error) {
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
	return response, nil
}

func preserveCurrentPatternSpeed(response *AssistantResponse, currentSpeed *int) {
	if currentSpeed != nil && response.Motion != nil && strings.TrimSpace(response.Motion.PatternID) != "" &&
		response.Motion.Intensity == nil && response.Motion.SpeedPercent == nil {
		speed := *currentSpeed
		response.Motion.SpeedPercent = &speed
	}
}

func validateAssistantResponse(response *AssistantResponse, patterns []PatternChoice, curation bool) error {
	response.Reply = strings.TrimSpace(response.Reply)
	if response.Reply == "" {
		return errors.New("assistant response reply is required")
	}
	if response.Motion == nil {
		return nil
	}

	response.Motion.Action = strings.ToLower(strings.TrimSpace(response.Motion.Action))
	response.Motion.PatternID = strings.ToLower(strings.TrimSpace(response.Motion.PatternID))
	response.Motion.Area = strings.ToLower(strings.TrimSpace(response.Motion.Area))
	switch response.Motion.Action {
	case MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionStop:
	default:
		return fmt.Errorf("unknown motion action %q", response.Motion.Action)
	}
	if response.Motion.PatternID != "" && !allowedPatternID(response.Motion.PatternID, patterns) {
		return fmt.Errorf("unknown motion pattern %q", response.Motion.PatternID)
	}
	if response.Motion.Area != "" && !oneOfZone(response.Motion.Area) {
		return fmt.Errorf("unknown motion area %q", response.Motion.Area)
	}
	if err := validateMotionRanges(*response.Motion); err != nil {
		return err
	}
	return validateMotionCombination(*response.Motion, curation)
}

func validateMotionRanges(command MotionCommand) error {
	if command.Intensity != nil && (*command.Intensity < 1 || *command.Intensity > 100) {
		return errors.New("motion intensity must be between 1 and 100")
	}
	if command.SpeedPercent != nil && (*command.SpeedPercent < 1 || *command.SpeedPercent > 100) {
		return errors.New("motion speed_percent must be between 1 and 100")
	}
	return nil
}

func validateMotionCombination(command MotionCommand, curation bool) error {
	if command.PatternID == "" && command.Intensity != nil {
		return errors.New("motion intensity requires an enabled pattern_id")
	}
	if command.PatternID != "" && curation && command.Intensity == nil && command.SpeedPercent == nil {
		return errors.New("curated pattern_id requires intensity")
	}
	if command.Intensity != nil && command.SpeedPercent != nil {
		return errors.New("motion cannot include both intensity and speed_percent")
	}
	if command.Action == MotionActionNone && (command.PatternID != "" || command.Intensity != nil || command.SpeedPercent != nil || command.Area != "") {
		return errors.New("motion action none cannot include target fields")
	}
	if command.Action == MotionActionStop && (command.PatternID != "" || command.Intensity != nil || command.SpeedPercent != nil || command.Area != "") {
		return errors.New("motion action stop cannot include target fields")
	}
	return nil
}

func oneOfZone(zone string) bool {
	for _, allowed := range AreaZones() {
		if zone == allowed {
			return true
		}
	}
	return false
}

func allowedPatternID(patternID string, patterns []PatternChoice) bool {
	for _, pattern := range patterns {
		if strings.EqualFold(strings.TrimSpace(pattern.ID), patternID) {
			return true
		}
	}
	return false
}

func defaultPatternChoices() []PatternChoice {
	return []PatternChoice{{ID: "stroke"}, {ID: "pulse"}, {ID: "tease"}}
}
