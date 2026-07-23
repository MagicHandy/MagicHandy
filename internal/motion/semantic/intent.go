package semantic

import (
	"fmt"
	"strings"
)

// ActionName is the director-classified physical action enum.
type ActionName string

const (
	ActionOral       ActionName = "oral"
	ActionHandjob    ActionName = "handjob"
	ActionRiding     ActionName = "riding"
	ActionTitjob     ActionName = "titjob"
	ActionDeepthroat ActionName = "deepthroat"
)

// LocationName is the director-classified stroke zone enum.
type LocationName string

const (
	LocationBase  LocationName = "base"
	LocationShaft LocationName = "shaft"
	LocationTip   LocationName = "tip"
	LocationFull  LocationName = "full"
)

// LLMIntent is the fast director JSON contract (enums only).
type LLMIntent struct {
	Action    ActionName   `json:"action"`
	Location  LocationName `json:"location"`
	Intensity int          `json:"intensity"`
}

// ValidateLLMIntent rejects unknown enums and out-of-range intensity.
func ValidateLLMIntent(intent LLMIntent) error {
	return NormalizeLLMIntent(&intent)
}

// NormalizeLLMIntent trims, lowercases, and validates director intent enums in place.
func NormalizeLLMIntent(intent *LLMIntent) error {
	if intent == nil {
		return fmt.Errorf("intent is required")
	}
	intent.Action = ActionName(strings.ToLower(strings.TrimSpace(string(intent.Action))))
	intent.Location = LocationName(strings.ToLower(strings.TrimSpace(string(intent.Location))))

	switch intent.Action {
	case ActionOral, ActionHandjob, ActionRiding, ActionTitjob, ActionDeepthroat:
	default:
		return fmt.Errorf("unknown action %q", intent.Action)
	}
	switch intent.Location {
	case LocationBase, LocationShaft, LocationTip, LocationFull:
	default:
		return fmt.Errorf("unknown location %q", intent.Location)
	}
	if intent.Intensity < 1 || intent.Intensity > 10 {
		return fmt.Errorf("intensity must be between 1 and 10, got %d", intent.Intensity)
	}
	return nil
}
