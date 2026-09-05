package httpapi

import (
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) contextualChatMotion(settings config.Settings, requests []string) chat.MotionContext {
	state := s.chatMotionContext(settings.Motion, settings.LLM)
	state.UserRequests = requests
	return state
}
func mapLayeredAutopilotCommand(command *chat.MotionCommand, settings config.MotionSettings, say string, next modes.TimingPreference) (modes.Decision, error) {
	if command.Layered == nil {
		return modes.Decision{}, errors.New("layered Autopilot requires a layered score")
	}
	if err := command.Layered.Validate(settings); err != nil {
		return modes.Decision{}, err
	}
	return modes.Decision{Segment: modes.Segment{Flow: motion.CloneFlowSpec(command.Layered), SpeedPercent: command.Layered.SpeedPercent}, Say: say, Next: next, Variability: modes.VariabilitySettled}, nil
}
