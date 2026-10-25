package httpapi

import (
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

func (s *Server) chatOutputContext(model config.LLMSettings) *chat.OutputContext {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(config.DeviceVibrator) {
		return nil
	}
	output := &chat.OutputContext{Coyote: settings.Device.Plays(config.DeviceOutputCoyote),
		Vibrator: &chat.VibrationContext{Follow: settings.Vibration.Follow, DepthEnd: settings.Vibration.DepthEnd}}
	for _, id := range settings.Device.PlaybackSet() {
		if id == config.DeviceOutputHandy || config.IsHandyConnectionID(id) {
			output.Strokers++
		}
	}
	output.Vibrator.Independent = model.IsHosted() && (model.MotionGenerationMode == config.LLMMotionModeLayered || model.MotionGenerationMode == config.LLMMotionModeCreativeV2)
	if engine := s.currentMotionEngine(); engine != nil {
		state := engine.Snapshot()
		if state.Running || state.Paused {
			output.Vibrator.Current = state.Target.Vibration
		}
	}
	if vibrator, err := s.currentVibrator(); err == nil {
		state := vibrator.Status()
		output.Vibrator.Ready = state.Selected && state.PlaybackState != "stale" && !s.vibratorRetired()
		output.Vibrator.Armed = state.EffectiveIntensityPercent > 0
		output.Vibrator.Motors = len(state.Motors)
	}
	return output
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
