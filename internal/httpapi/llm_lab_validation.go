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
	return validateLabAutopilot(trial)
}

// validateLabAutopilot keeps the comparison methods' conservative continuation
// rule. The continuous methods match production: the model judges which recent
// requests still apply, and saved limits bound what it may choose.
func validateLabAutopilot(trial chat.LLMLabTrial) chat.LLMLabTrial {
	if !trial.Autopilot || !trial.Valid || trial.Method == "layered" || trial.Method == "creative_v2" || chat.IsStrokeLabMethod(trial.Method) {
		return trial
	}
	if !labAutopilotWithinRequest(trial.Before, trial.After) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot cannot increase speed or widen the requested band."
	}
	return trial
}
