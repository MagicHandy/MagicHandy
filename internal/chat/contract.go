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

// Mood is model-reported reply-register state. It is inert metadata and never
// enters MotionCommand, MotionContext, or transport dispatch.
type Mood string

// Moods match the reviewed STGPT-RV register and remain stable JSON protocol
// values even when surrounding prompt prose is localized.
const (
	MoodCurious      Mood = "Curious"
	MoodTeasing      Mood = "Teasing"
	MoodPlayful      Mood = "Playful"
	MoodLoving       Mood = "Loving"
	MoodExcited      Mood = "Excited"
	MoodPassionate   Mood = "Passionate"
	MoodSeductive    Mood = "Seductive"
	MoodAnticipatory Mood = "Anticipatory"
	MoodBreathless   Mood = "Breathless"
	MoodDominant     Mood = "Dominant"
	MoodSubmissive   Mood = "Submissive"
	MoodVulnerable   Mood = "Vulnerable"
	MoodConfident    Mood = "Confident"
	MoodIntimate     Mood = "Intimate"
	MoodNeedy        Mood = "Needy"
	MoodOverwhelmed  Mood = "Overwhelmed"
	MoodAfterglow    Mood = "Afterglow"
)

var moodValues = []Mood{
	MoodCurious, MoodTeasing, MoodPlayful, MoodLoving, MoodExcited,
	MoodPassionate, MoodSeductive, MoodAnticipatory, MoodBreathless,
	MoodDominant, MoodSubmissive, MoodVulnerable, MoodConfident,
	MoodIntimate, MoodNeedy, MoodOverwhelmed, MoodAfterglow,
}

// Moods returns the accepted mood values in prompt order.
func Moods() []Mood {
	return append([]Mood(nil), moodValues...)
}

// AssistantResponse is the only model output shape accepted by MagicHandy.
type AssistantResponse struct {
	Reply   string         `json:"reply"`
	NewMood *Mood          `json:"new_mood,omitempty"`
	Motion  *MotionCommand `json:"motion,omitempty"`
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

type unknownPatternError struct {
	patternID string
}

func (e unknownPatternError) Error() string {
	return fmt.Sprintf("unknown motion pattern %q", e.patternID)
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
	// Fields the active prompt did not advertise are inert model noise. Strip
	// them before validation so utility chat does not repair an unused mood.
	enforceCapabilities(&response, capabilities)
	if err := validateAssistantMood(&response); err != nil {
		return AssistantResponse{}, err
	}
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

// resolveOverspecifiedPacing picks one pacing representation when the model sent
// both, instead of discarding the whole decision.
//
// The contract asks for exactly one of pattern_id+intensity or speed_percent,
// and rejecting the pair is right for a hand-written command. For a model it was
// costing the entire turn: measured against the live 26B, 60% of autopilot
// motion decisions were thrown away on this rule alone, and every one of them
// fell back to the deterministic planner. That is what flattened session speed.
//
// Both values here came from the model and both remain subject to the band and
// range checks below, so nothing is invented and no limit moves. The choice
// follows the contract's own preference: a named pattern is paced by intensity,
// and intensity without a pattern is meaningless, so speed_percent wins there.
// This mirrors enforceCapabilities, which already strips stray fields rather
// than spending a repair round-trip on model noise.
func resolveOverspecifiedPacing(motion *MotionCommand) {
	if motion == nil || motion.Intensity == nil || motion.SpeedPercent == nil {
		return
	}
	if strings.TrimSpace(motion.PatternID) != "" {
		motion.SpeedPercent = nil
		return
	}
	motion.Intensity = nil
}

func validateAssistantResponse(response *AssistantResponse, patterns []PatternChoice, curation bool) error {
	response.Reply = strings.TrimSpace(response.Reply)
	if response.Reply == "" {
		return errors.New("assistant response reply is required")
	}
	if err := validateAssistantMood(response); err != nil {
		return err
	}
	if response.Motion == nil {
		return nil
	}

	response.Motion.Action = strings.ToLower(strings.TrimSpace(response.Motion.Action))
	response.Motion.PatternID = strings.ToLower(strings.TrimSpace(response.Motion.PatternID))
	response.Motion.Area = strings.ToLower(strings.TrimSpace(response.Motion.Area))
	resolveOverspecifiedPacing(response.Motion)
	switch response.Motion.Action {
	case MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionStop:
	default:
		return fmt.Errorf("unknown motion action %q", response.Motion.Action)
	}
	if response.Motion.PatternID != "" && !allowedPatternID(response.Motion.PatternID, patterns) {
		return unknownPatternError{patternID: response.Motion.PatternID}
	}
	if response.Motion.Area != "" && !oneOfZone(response.Motion.Area) {
		return fmt.Errorf("unknown motion area %q", response.Motion.Area)
	}
	if err := validateMotionRanges(*response.Motion); err != nil {
		return err
	}
	return validateMotionCombination(*response.Motion, curation)
}

func validateAssistantMood(response *AssistantResponse) error {
	if response.NewMood == nil {
		return nil
	}
	if _, ok := validMood(*response.NewMood); !ok {
		return fmt.Errorf("unknown assistant mood %q", *response.NewMood)
	}
	return nil
}

func validMood(value Mood) (Mood, bool) {
	for _, allowed := range moodValues {
		if value == allowed {
			return allowed, true
		}
	}
	return "", false
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
