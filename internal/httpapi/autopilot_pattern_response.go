package httpapi

import (
	"fmt"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"strings"
)

func (s *Server) mapPatternAutopilotCommand(command *chat.MotionCommand, input modes.DecisionInput, say string, next modes.TimingPreference, variability modes.VariabilityPreference) (modes.Decision, error) {
	patternID := strings.TrimSpace(command.PatternID)
	if patternID == "" {
		patternID = string(input.CurrentPatternID)
	}
	speed := input.CurrentSpeed
	if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	} else if command.Intensity != nil {
		speed = *command.Intensity
	}
	var areaFocus *motion.AreaFocus
	if input.CurrentAreaFocus != nil {
		focus := *input.CurrentAreaFocus
		areaFocus = &focus
	}
	if command.Area != "" {
		focus, ok := zoneAreaFocus(command.Area)
		if !ok {
			return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
		}
		areaFocus = focus
	}
	if patternID == "" || speed <= 0 {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}
	if strings.EqualFold(patternID, string(input.CurrentPatternID)) &&
		speed == input.CurrentSpeed && sameAreaFocus(areaFocus, input.CurrentAreaFocus) {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}

	resolved, found, err := s.patterns.ResolveEnabled(patternID)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve Autopilot pattern: %w", err)
	}
	if !found {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}
	return modes.Decision{
		Segment: modes.Segment{
			PatternID:    motion.PatternID(resolved.ID),
			SpeedPercent: speed,
			AreaFocus:    areaFocus,
		},
		Pattern:     &resolved,
		Say:         say,
		Next:        next,
		Variability: variability,
	}, nil
}
