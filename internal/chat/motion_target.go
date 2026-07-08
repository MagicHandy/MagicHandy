package chat

import (
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// MotionTargetFromCommand maps a validated motion command to an engine target.
func MotionTargetFromCommand(command *MotionCommand, current motion.ActiveMotionState, generationMode string) motion.MotionTarget {
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
		return proceduralMotionTarget(command, current)
	}
}

func proceduralMotionTarget(command *MotionCommand, current motion.ActiveMotionState) motion.MotionTarget {
	patternID := motion.PatternID(command.PatternID)
	speedPercent := 0
	if command.SpeedPercent != nil {
		speedPercent = *command.SpeedPercent
	}
	if command.IntensidadeLegacy != "" {
		speedPercent = intensidadeToSpeed(command.IntensidadeLegacy)
	}
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
	return motion.MotionTarget{
		Label:        "Chat",
		Source:       "chat",
		PatternID:    patternID,
		SpeedPercent: speedPercent,
	}
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
