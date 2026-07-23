package chat

import (
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

// MotionTargetFromCommand maps a validated motion command to semantic engine metadata.
// Procedural chat motion is dispatched via manualqueue.Player; this target is used for
// library mode engine retargeting and diagnostics (traces, UI) when procedural fields are present.
func MotionTargetFromCommand(command *MotionCommand, current motion.ActiveMotionState, generationMode string) motion.MotionTarget {
	return MotionTargetFromCommandWithPreferences(command, current, generationMode, semantic.DefaultMotionPreferences())
}

// MotionTargetFromCommandWithPreferences maps procedural bounds via user motion preferences.
func MotionTargetFromCommandWithPreferences(
	command *MotionCommand,
	current motion.ActiveMotionState,
	generationMode string,
	prefs semantic.MotionPreferences,
) motion.MotionTarget {
	if command == nil {
		return motion.MotionTarget{Label: "Chat", Source: "chat"}
	}

	switch normalizeMotionGenerationMode(generationMode) {
	case config.MotionGenerationModeLibrary:
		return motion.MotionTarget{
			Label:        "Chat",
			Source:       "chat",
			PatternID:    libraryTagToPatternID(command.PadraoID),
			SpeedPercent: libraryTagToSpeed(command.PadraoID),
		}
	default:
		return proceduralMotionTarget(command, current, prefs)
	}
}

func proceduralMotionTarget(command *MotionCommand, current motion.ActiveMotionState, prefs semantic.MotionPreferences) motion.MotionTarget {
	target := motion.MotionTarget{
		Label:  "Chat",
		Source: "chat",
	}

	speedPercent := command.Velocidade
	if speedPercent == 0 && command.SpeedPercent != nil {
		speedPercent = *command.SpeedPercent
	}
	if speedPercent == 0 && command.IntensidadeLegacy != "" {
		speedPercent = intensidadeToSpeed(command.IntensidadeLegacy)
	}

	patternID := motion.PatternID(command.PatternID)
	if command.Estilo != "" {
		patternID = estiloToPatternID(command.Estilo)
	}

	if current.Running {
		if patternID == "" {
			patternID = current.Target.PatternID
		}
		if speedPercent == 0 {
			speedPercent = current.Target.SpeedPercent
		}
	}

	target.PatternID = patternID
	target.SpeedPercent = speedPercent
	target.AreaFocus = proceduralAreaFocus(command, prefs)
	return target
}

func proceduralAreaFocus(command *MotionCommand, prefs semantic.MotionPreferences) *motion.AreaFocus {
	if command == nil {
		return nil
	}
	if len(command.StrokeRange) == 2 {
		minPct := int(command.StrokeRange[0] * 100)
		maxPct := int(command.StrokeRange[1] * 100)
		if minPct > maxPct {
			minPct, maxPct = maxPct, minPct
		}
		return &motion.AreaFocus{MinPercent: minPct, MaxPercent: maxPct}
	}
	if command.Regiao == "" {
		return nil
	}
	if min, max, ok := semantic.BoundsFromRegiao(command.Regiao, prefs); ok {
		return &motion.AreaFocus{
			MinPercent: int(min * 100),
			MaxPercent: int(max * 100),
		}
	}
	minPct, maxPct, ok := motion.RegionBounds(command.Regiao)
	if !ok {
		return nil
	}
	return &motion.AreaFocus{MinPercent: minPct, MaxPercent: maxPct}
}

func intensidadeToSpeed(value string) int {
	switch value {
	case "baixa":
		return 30
	case "media":
		return 50
	case "alta":
		return 75
	case "caos":
		return 90
	default:
		return 50
	}
}

func estiloToPatternID(value string) motion.PatternID {
	switch value {
	case "vibrato":
		return motion.PatternPulse
	case "pulsante":
		return motion.PatternTease
	default:
		return motion.PatternStroke
	}
}

func libraryTagToPatternID(tag string) motion.PatternID {
	switch tag {
	case "rapido_curto", "vibrato_intenso", "pulsante", "caotico", "aleatorio":
		return motion.PatternPulse
	case "lento_profundo":
		return motion.PatternStroke
	default:
		return motion.PatternStroke
	}
}

func libraryTagToSpeed(tag string) int {
	switch tag {
	case "rapido_curto", "caotico", "vibrato_intenso":
		return 80
	case "lento_profundo":
		return 35
	case "pulsante":
		return 55
	case "aleatorio":
		return 65
	default:
		return 50
	}
}
