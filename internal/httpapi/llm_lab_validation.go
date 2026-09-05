package httpapi

import (
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) validateLabTrial(trial chat.LLMLabTrial, state llmLabState) chat.LLMLabTrial {
	latestSettings, _ := s.store.Snapshot()
	if motion.LabSettingsKey(latestSettings.Motion) != state.SettingsKey {
		trial.Valid, trial.After, trial.Error = false, state.Current, "motion limits changed during generation; retry the trial"
	}
	if trial.Valid {
		if _, err := motion.FlowTarget(trial.After, latestSettings.Motion); err != nil {
			trial.Valid, trial.After, trial.Error = false, state.Current, err.Error()
		}
	}
	if !trial.Valid {
		trial.Changed = []string{}
	}
	return validateLabAutopilot(trial, labHumanRequests(state.DirectiveTurns))
}

func validateLabAutopilot(trial chat.LLMLabTrial, requests []string) chat.LLMLabTrial {
	if !trial.Autopilot || !trial.Valid {
		return trial
	}
	guided := chat.HasMotionDirection(requests)
	continuous := trial.Method == "layered" || trial.Method == "creative_v2"
	if (!continuous || guided) && !labAutopilotWithinRequest(trial.Before, trial.After) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot cannot increase speed or widen the requested band."
	}
	if trial.Method == "creative_v2" && trial.Valid && guided && !chat.CreativeV2CharacterUnchanged(trial.Before, trial.After) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Creative v2 Autopilot changed the requested character."
	}
	if continuous && trial.Valid && len(trial.Changed) > 0 && chat.LayeredExactHoldRequested(requests) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot changed an explicitly fixed score."
	}

	return trial
}
