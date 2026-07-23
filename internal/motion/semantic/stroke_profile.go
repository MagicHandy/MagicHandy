package semantic

import "github.com/mapledaemon/MagicHandy/internal/motion"

// ResolveStrokeProfile maps director action to asymmetric stroke timing.
func ResolveStrokeProfile(intent LLMIntent) motion.StrokeProfile {
	switch intent.Action {
	case ActionRiding:
		return motion.NormalizeStrokeProfile(motion.StrokeProfile{
			DownstrokeRatio: 0.35,
			UpstrokeRatio:   0.65,
		})
	case ActionDeepthroat:
		return motion.NormalizeStrokeProfile(motion.StrokeProfile{
			DownstrokeRatio: 0.30,
			UpstrokeRatio:   0.70,
			HasBottomBounce: true,
		})
	default:
		return motion.DefaultStrokeProfile()
	}
}
