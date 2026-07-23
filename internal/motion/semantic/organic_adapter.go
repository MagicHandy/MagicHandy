package semantic

import (
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// OrganicConfigFromIntent builds organic stroke parameters from director intent.
//
// Example: location tip with default prefs confines waves to roughly 70–100% stroke.
func OrganicConfigFromIntent(intent LLMIntent, prefs MotionPreferences, velocity int) (motion.OrganicConfig, error) {
	min, max, err := ResolveMotionBounds(intent, prefs)
	if err != nil {
		return motion.OrganicConfig{}, err
	}
	physics := motion.ChaoticPhysics{
		Velocidade:     velocity,
		Intensidade:    intent.Intensity * 10,
		Regiao:         LocationToRegiao(intent.Location),
		TipoBatida:     "fluido",
		StrokeRangeMin: min,
		StrokeRangeMax: max,
		Action:         string(intent.Action),
		StrokeProfile:  ResolveStrokeProfile(intent),
	}
	return motion.OrganicConfigFromPhysics(physics), nil
}
