package semantic

import (
	"fmt"
	"math"
)

// ResolveMotionBounds maps director intent and user prefs to normalized stroke bounds.
func ResolveMotionBounds(intent LLMIntent, prefs MotionPreferences) (min, max float64, err error) {
	if err := ValidateLLMIntent(intent); err != nil {
		return 0, 0, err
	}
	prefs = NormalizeMotionPreferences(prefs)

	zone := zoneForIntent(intent, prefs)
	bounds, ok := prefs.Zones[zone]
	if !ok {
		return 0, 0, fmt.Errorf("zone %q is not configured", zone)
	}
	min = clamp01(bounds.Min)
	max = clamp01(bounds.Max)
	if min >= max {
		return 0, 0, fmt.Errorf("zone %q has invalid range %.2f..%.2f", zone, min, max)
	}
	return min, max, nil
}

func zoneForIntent(intent LLMIntent, prefs MotionPreferences) ZoneName {
	if override, ok := prefs.ActionOverrides[intent.Action]; ok {
		return override
	}
	return ZoneFromLocation(intent.Location)
}

// ZoneFromLocation maps a director location enum to a preference zone name.
func ZoneFromLocation(location LocationName) ZoneName {
	switch location {
	case LocationBase:
		return ZoneBase
	case LocationShaft:
		return ZoneShaft
	case LocationTip:
		return ZoneTip
	case LocationFull:
		return ZoneFull
	default:
		return ZoneFull
	}
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
