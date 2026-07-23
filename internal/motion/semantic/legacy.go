package semantic

import "strings"

// BoundsFromRegiao maps legacy procedural regiao strings to normalized stroke bounds.
// Used on the legacy chat path before Director mode is enabled.
func BoundsFromRegiao(regiao string, prefs MotionPreferences) (min, max float64, ok bool) {
	regiao = strings.ToLower(strings.TrimSpace(regiao))
	if regiao == "" {
		return 0, 0, false
	}
	location := RegiaoToLocation(regiao)
	prefs = NormalizeMotionPreferences(prefs)
	zone := ZoneFromLocation(location)
	bounds, exists := prefs.Zones[zone]
	if !exists {
		return 0, 0, false
	}
	min = clamp01(bounds.Min)
	max = clamp01(bounds.Max)
	if min >= max {
		return 0, 0, false
	}
	return min, max, true
}

// LocationToRegiao maps director location enums to legacy procedural regiao strings.
func LocationToRegiao(location LocationName) string {
	switch location {
	case LocationBase:
		return "base"
	case LocationShaft:
		return "meio"
	case LocationTip:
		return "cabeca"
	case LocationFull:
		return "full"
	default:
		return "full"
	}
}

// RegiaoToLocation maps legacy regiao strings to director location enums.
func RegiaoToLocation(regiao string) LocationName {
	switch strings.ToLower(strings.TrimSpace(regiao)) {
	case "base", "meio_base":
		return LocationBase
	case "meio", "meio_cabeca":
		return LocationShaft
	case "cabeca":
		return LocationTip
	case "full", "completo", "cabeca_base", "aleatoria":
		return LocationFull
	default:
		return LocationFull
	}
}
