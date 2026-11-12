package httpapi

import (
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) chatOutputContext(model config.LLMSettings) *chat.OutputContext {
	settings, _ := s.store.Snapshot()
	vibrator, coyote := settings.Device.Plays(config.DeviceVibrator), settings.Device.Plays(config.DeviceOutputCoyote)
	paceOnly := settings.Device.Plays(config.DeviceVacuGlide)
	if !vibrator && !coyote && !paceOnly {
		return nil
	}
	output := &chat.OutputContext{Coyote: coyote}
	output.Strokers, output.PaceOnly = countStrokers(settings.Device.PlaybackSet())
	if coyote {
		output.Estim = s.chatEstimContext(settings)
	}
	if paceOnly {
		output.Tempo = s.chatTempoContext(settings)
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

// countStrokers counts the playing strokers, and how many of them follow only
// pace: a VacuGlide makes its own strokes (ADR 0069).
func countStrokers(playback []string) (strokers, paceOnly int) {
	for _, id := range playback {
		switch {
		case id == config.DeviceOutputHandy || id == config.DeviceAutoblow || config.IsHandyConnectionID(id):
			strokers++
		case id == config.DeviceVacuGlide:
			strokers++
			paceOnly++
		}
	}
	return strokers, paceOnly
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
	estim.SilentReason = status.SilentReason
	return estim
}

func (s *Server) vibratorRetired() bool {
	return s.playbackDeviceRetired(config.DeviceVibrator)
}

func (s *Server) playbackDeviceRetired(deviceID string) bool {
	engine := s.currentMotionEngine()
	if engine == nil {
		return false
	}
	state := engine.Snapshot()
	if !state.Running || state.LinkedSession == nil {
		return false
	}
	for _, id := range state.LinkedSession.RetiredDevices {
		if id == deviceID {
			return true
		}
	}
	return false
}

// chatTempoContext reports the VacuGlide's tempo for the model: what plays,
// and whether the person lets motion change the air (ADR 0071).
func (s *Server) chatTempoContext(settings config.Settings) *chat.TempoContext {
	tempo := &chat.TempoContext{Leads: tempoLeads(settings.Device), AirAllowed: settings.VacuGlide.MotionAir}
	if owner := s.autoblow.vacuglide; owner != nil {
		status := owner.Status()
		tempo.Ready = status.Connected && status.PlaybackState != "stale" && !s.playbackDeviceRetired(config.DeviceVacuGlide)
		tempo.AirSteps = status.MotionAirSteps
	}
	if engine := s.currentMotionEngine(); engine != nil {
		state := engine.Snapshot()
		if (state.Running || state.Paused) && state.Target.Tempo != nil {
			current := state.Target.Tempo.Clone()
			tempo.Current = &current
		} else if state.Target.Tempo != nil {
			tempo.Previous = motion.CloneTempoSpec(state.Target.Tempo)
		}
		tempo.Running = state.Running && !state.Paused
		if state.Tempo != nil {
			tempo.Level = state.Tempo.LevelPercent
		}
	}
	return tempo
}
