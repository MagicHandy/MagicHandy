package semantic

import (
	"encoding/json"
)

// ZoneName identifies a normalized stroke band on the 0..1 axis.
type ZoneName string

const (
	ZoneBase  ZoneName = "base"
	ZoneShaft ZoneName = "shaft"
	ZoneTip   ZoneName = "tip"
	ZoneFull  ZoneName = "full"
)

// ZoneRange is a normalized stroke window on the Handy axis.
type ZoneRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// MotionPreferences stores user zoning and action override rules.
type MotionPreferences struct {
	Zones           map[ZoneName]ZoneRange   `json:"zones"`
	ActionOverrides map[ActionName]ZoneName `json:"action_overrides,omitempty"`
}

// DefaultMotionPreferences returns documented factory zoning defaults.
func DefaultMotionPreferences() MotionPreferences {
	return MotionPreferences{
		Zones: map[ZoneName]ZoneRange{
			ZoneBase:  {Min: 0.0, Max: 0.3},
			ZoneShaft: {Min: 0.3, Max: 0.7},
			ZoneTip:   {Min: 0.7, Max: 1.0},
			ZoneFull:  {Min: 0.0, Max: 1.0},
		},
		ActionOverrides: map[ActionName]ZoneName{
			ActionDeepthroat: ZoneFull,
			ActionOral:       ZoneFull,
		},
	}
}

// NormalizeMotionPreferences fills missing zones and overrides from defaults.
func NormalizeMotionPreferences(prefs MotionPreferences) MotionPreferences {
	defaults := DefaultMotionPreferences()
	if prefs.Zones == nil {
		prefs.Zones = map[ZoneName]ZoneRange{}
	}
	for zone, bounds := range defaults.Zones {
		if _, ok := prefs.Zones[zone]; !ok {
			prefs.Zones[zone] = bounds
		}
	}
	if prefs.ActionOverrides == nil {
		prefs.ActionOverrides = map[ActionName]ZoneName{}
	}
	for action, zone := range defaults.ActionOverrides {
		if _, ok := prefs.ActionOverrides[action]; !ok {
			prefs.ActionOverrides[action] = zone
		}
	}
	return prefs
}

// MarshalMotionPreferences serializes prefs for the settings document blob.
func MarshalMotionPreferences(prefs MotionPreferences) (json.RawMessage, error) {
	prefs = NormalizeMotionPreferences(prefs)
	return json.Marshal(prefs)
}

// LoadMotionPreferences unmarshals settings JSON or returns defaults when empty.
func LoadMotionPreferences(raw json.RawMessage) MotionPreferences {
	if len(raw) == 0 {
		return DefaultMotionPreferences()
	}
	var prefs MotionPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return DefaultMotionPreferences()
	}
	return NormalizeMotionPreferences(prefs)
}

// DefaultMotionPreferencesJSON returns the factory prefs blob for settings migration.
func DefaultMotionPreferencesJSON() json.RawMessage {
	raw, err := MarshalMotionPreferences(DefaultMotionPreferences())
	if err != nil {
		return nil
	}
	return raw
}
