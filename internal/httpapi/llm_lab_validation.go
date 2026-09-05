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
	if trial.Autopilot && trial.Valid && !labAutopilotWithinRequest(trial.Before, trial.After) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot cannot increase speed or widen the requested band."
	}
	if trial.Autopilot && trial.Method == "layered" && trial.Valid && len(trial.Changed) > 0 && chat.LayeredExactHoldRequested(labHumanRequests(state.DirectiveTurns)) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot changed an explicitly fixed score."
	}

	return trial
}
