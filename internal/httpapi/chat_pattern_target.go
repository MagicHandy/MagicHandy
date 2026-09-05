package httpapi

import (
	"fmt"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) patternChatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState) (motion.MotionTarget, error) {
	patternID := motion.PatternID(command.PatternID)
	speedPercent := 0
	if command.SpeedPercent != nil {
		speedPercent = *command.SpeedPercent
	} else if command.Intensity != nil {
		speedPercent = *command.Intensity
	}
	var definition *motion.PatternDefinition
	var programDefinition *motion.ProgramDefinition
	var programID string
	if patternID != "" {
		resolved, ok, err := s.patterns.ResolveEnabled(string(patternID))
		if err != nil {
			return motion.MotionTarget{}, fmt.Errorf("resolve chat pattern: %w", err)
		}
		if ok {
			definition = &resolved
		} else {
			// The enabled set may change while the model is streaming. Never apply
			// a now-disabled selection; fall back to the deterministic target.
			patternID = ""
		}
	}
	if current.Running {
		if patternID == "" {
			if current.Target.Program != nil {
				copied := *current.Target.Program
				copied.Points = append([]motion.CurvePoint(nil), current.Target.Program.Points...)
				programDefinition = &copied
				programID = current.Target.ProgramID
			} else {
				patternID = current.Target.PatternID
			}
			if programDefinition == nil && current.Target.Pattern != nil {
				copied := *current.Target.Pattern
				copied.Points = append([]motion.CurvePoint(nil), current.Target.Pattern.Points...)
				copied.Tags = append([]string(nil), current.Target.Pattern.Tags...)
				definition = &copied
			}
		}
		if speedPercent == 0 {
			speedPercent = current.Target.SpeedPercent
		}
	}
	return motion.MotionTarget{
		Label:        "Chat",
		Source:       "chat",
		PatternID:    patternID,
		ProgramID:    programID,
		SpeedPercent: speedPercent,
		Pattern:      definition,
		Program:      programDefinition,
		AreaFocus:    resolveAreaFocus(command.Area, current),
	}, nil
}
