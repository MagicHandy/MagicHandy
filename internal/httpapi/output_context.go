package httpapi

import (
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

func (s *Server) chatOutputContext(model config.LLMSettings) *chat.OutputContext {
	settings, _ := s.store.Snapshot()
	vibrator, coyote := settings.Device.Plays(config.DeviceVibrator), settings.Device.Plays(config.DeviceOutputCoyote)
	if !vibrator && !coyote {
		return nil
	}
	output := &chat.OutputContext{Coyote: coyote}
	for _, id := range settings.Device.PlaybackSet() {
		if id == config.DeviceOutputHandy || config.IsHandyConnectionID(id) {
			output.Strokers++
		}
	}
	if coyote {
		output.Estim = s.chatEstimContext(settings)
	}
	if !vibrator {
		return output
	}
	output.Vibrator = &chat.VibrationContext{Follow: settings.Vibration.Follow, DepthEnd: settings.Vibration.DepthEnd}
	features, modelSets := model.ModelFeatures(), settings.Vibration.ModelSetsLevel()
	output.Vibrator.Independent = features.IndependentVibration && modelSets
	output.Vibrator.LevelControl = features.VibrationLevel && modelSets
	if engine := s.currentMotionEngine(); engine != nil {
		state := engine.Snapshot()
		if state.Running || state.Paused {
			output.Vibrator.Current = state.Target.Vibration
		} else if output.Vibrator.Independent {
			// Saved data for an explicit continuation, never a playing envelope
			// or authority for autonomous work to cross Stop.
			output.Vibrator.Previous = state.Target.Vibration
		}
	}
	if vibrator, err := s.currentVibrator(); err == nil {
		state := vibrator.Status()
		// A Max below one motor step is as silent as a missing device.
		output.Vibrator.Ready = state.Selected && state.PlaybackState != "stale" && !s.vibratorRetired() && state.EffectiveCeilingPercent > 0
		output.Vibrator.Stopped = state.Suspended
		output.Vibrator.LevelPercent = state.ModelLevelPercent
		output.Vibrator.LevelFollowsHandy = state.LevelSource != "model"
		output.Vibrator.PreviousLevelPercent = state.PreviousLevelPercent
		output.Vibrator.Motors = len(state.Motors)
	}
	return output
}

// chatEstimContext reports the Coyote's facts for the model: never its
// strength numbers or any control, only whether it can be felt (ADR 0063).
func (s *Server) chatEstimContext(settings config.Settings) *chat.EstimContext {
	estim := &chat.EstimContext{Follows: settings.Estim.Follow}
	if s.coyote.owner == nil {
		return estim
	}
	status := s.coyote.owner.Status()
	estim.Connected = status.Connected
	// The same backend reason the panel shows: never claimed from a commanded
	// zero the device has not reported.
	estim.Silent = status.SilentReason != ""
	return estim
}

func (s *Server) vibratorRetired() bool {
	engine := s.currentMotionEngine()
	if engine == nil {
		return false
	}
	state := engine.Snapshot()
	if !state.Running || state.LinkedSession == nil {
		return false
	}
	for _, id := range state.LinkedSession.RetiredDevices {
		if id == config.DeviceVibrator {
			return true
		}
	}
	return false
}
